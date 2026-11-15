// Package search answers concept queries by scanning the bundle and scoring
// what it finds, with no persisted index.
//
// The Bleve index this replaces cost 13MB for 1.4MB of markdown and, more to
// the point, was a second source of truth: a deleted concept stayed in results
// until something pruned it, and a lost index stayed lost until something
// happened to rebuild it. Scanning cannot desynchronize.
// Measured on a 918-concept bundle, a full-text pass takes about 17ms and a
// frontmatter filter about 10ms — far below the point where a human or an
// agent notices, and OKF section 3.1 anticipates exactly this ("a consumer
// that wants a tag-browsing view can synthesize one at consumption time by
// scanning frontmatter").
//
// What is deliberately kept is *ranking*. The dominant agent pattern is search
// then read, with a small result cap, so the caller needs the best few hits
// rather than the first few in directory order. Scoring is a weighted field
// match rather than BM25 — see scoreConcept in pkg/bundle, which owns the walk
// and the ranking so that the public package ranks as `okfw search` does. This
// package adds the filters, the paging and the CLI's output.
//
// The cost is linear in bundle size. At tens of thousands of concepts this is
// the design choice to revisit.
package search

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/infrashift/okf-wiki/internal/config"
	okfbundle "github.com/infrashift/okf-wiki/pkg/bundle"
)

// Options selects and filters concepts. The zero value matches everything.
type Options struct {
	Query   string   // free-text query; empty means "filters only"
	Project string   // exact project match
	Type    string   // OKF free-form type
	Kind    string   // structured kind: note/daily-note/issue
	Tags    []string // all must be present
	Dir     string   // bundle subdirectory to search, recursively; empty = all
	Limit   int      // max results (0 → DefaultLimit)
	Offset  int      // pagination offset

	Status string // OKF lifecycle: draft/stable/deprecated
	Stale  bool   // only concepts whose stale_after has passed
}

// DefaultLimit caps a result set that did not ask for a size.
const DefaultLimit = 20

// Result is one hit.
type Result struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Kind        string   `json:"kind"`
	Description string   `json:"description,omitempty"`
	Project     string   `json:"project"`
	Tags        []string `json:"tags"`
	FilePath    string   `json:"file_path"`
	Score       float64  `json:"score"`
	Snippet     string   `json:"snippet,omitempty"`

	Status     string `json:"status,omitempty"`
	StaleAfter string `json:"stale_after,omitempty"`

	// Derived, so a caller can weigh a hit without opening it: a search that
	// returned a deprecated, stale or never-reviewed concept looked exactly like
	// one that returned the reviewed, current answer.
	Trust string `json:"trust"` // OKF section 5.3: unverified, machine-confirmed or human-reviewed
	Stale bool   `json:"stale"` // stale_after has passed (config.IsStale)
}

// Lifecycle is the hit's OKF lifecycle status, with absent read as stable.
func (r Result) Lifecycle() string { return lifecycleOf(r.Status) }

// unbounded lets the CLI read a bundle of any size, as it always has: the
// limits in okfbundle.Limits protect a consumer of untrusted bundles, and the
// CLI reads the user's own.
var unbounded = okfbundle.Limits{MaxFileBytes: math.MaxInt64, MaxFiles: math.MaxInt}

// Search walks the bundle, keeps concepts passing every filter, scores them
// against the query and returns one page plus the total number of matches.
//
// The walk and the ranking are pkg/bundle's Walk and Search, the code a
// consumer of the public package runs, so the two cannot drift apart.
func Search(bundleRoot string, opts Options) ([]Result, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	now := time.Now()

	dir, err := scopeDir(bundleRoot, opts.Dir)
	if err != nil {
		return nil, 0, err
	}

	var candidates []*okfbundle.Concept
	byConcept := make(map[*okfbundle.Concept]Result)
	err = okfbundle.Walk(os.DirFS(bundleRoot), dir, unbounded, func(rel string, content []byte) error {
		fm, err := config.ParseFrontmatter(content)
		if err != nil {
			return nil // no parseable frontmatter: not a concept
		}
		abs, _ := filepath.Abs(filepath.Join(bundleRoot, filepath.FromSlash(rel)))
		r := Result{
			// A concept okfw did not write may carry no id; its OKF concept ID
			// (the path) is what identifies it, and what read and resolve accept.
			ID:          config.ConceptKey(fm, rel),
			Title:       fm.Title,
			Type:        fm.Type,
			Kind:        fm.GetKind(),
			Description: fm.Description,
			Project:     fm.Project,
			Tags:        fm.Tags,
			FilePath:    abs,
			Status:      fm.Status,
			StaleAfter:  fm.StaleAfter,
			Trust:       fm.TrustTier(),
			Stale:       config.IsStale(fm.StaleAfter, now),
		}
		if !matches(r, opts) {
			return nil
		}
		c := &okfbundle.Concept{
			Path:        rel,
			ID:          r.ID,
			Title:       r.Title,
			Type:        r.Type,
			Description: r.Description,
			Tags:        r.Tags,
			Status:      fm.LifecycleStatus(),
			Body:        config.ExtractBody(content),
		}
		candidates = append(candidates, c)
		byConcept[c] = r
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	hits := okfbundle.Search(candidates, opts.Query, 0)
	total := len(hits)
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Offset >= total {
		return nil, total, nil
	}
	end := min(opts.Offset+limit, total)
	page := make([]Result, 0, end-opts.Offset)
	for _, h := range hits[opts.Offset:end] {
		r := byConcept[h.Concept]
		r.Score, r.Snippet = h.Score, h.Snippet
		page = append(page, r)
	}
	return page, total, nil
}

// scopeDir resolves Options.Dir to the directory the walk starts from. The
// walk starting there, rather than filtering every path afterwards, is what
// makes a scoped search cost the scope and not the bundle. The dir comes from
// an MCP client unvetted, so it must stay inside the bundle.
//
// It returns the directory as okfbundle.Walk takes it: slash-separated and
// relative to the bundle root, "." for the whole bundle.
func scopeDir(bundleRoot, dir string) (string, error) {
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	if dir == "" {
		return ".", nil
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == ".." || seg == "." || strings.HasPrefix(seg, ".") {
			return "", fmt.Errorf("invalid dir %q: must be a directory inside the wiki", dir)
		}
	}
	start := filepath.Join(bundleRoot, filepath.FromSlash(dir))
	if fi, err := os.Stat(start); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("no directory %q in the wiki", dir)
	}
	return path.Clean(dir), nil
}

func lifecycleOf(s string) string {
	// OKF section 5.4: an absent status means stable.
	switch s {
	case "draft", "stable", "deprecated":
		return s
	}
	return "stable"
}

// matches applies every non-query filter.
func matches(r Result, opts Options) bool {
	if opts.Project != "" && r.Project != opts.Project {
		return false
	}
	if opts.Type != "" && r.Type != opts.Type {
		return false
	}
	if opts.Kind != "" && r.Kind != opts.Kind {
		return false
	}
	if opts.Status != "" && lifecycleOf(r.Status) != opts.Status {
		return false
	}
	if opts.Stale && !r.Stale {
		return false
	}
	for _, want := range opts.Tags {
		found := false
		for _, have := range r.Tags {
			if have == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ResultsToJSON renders results as a JSON array.
func ResultsToJSON(results []Result) (string, error) {
	if results == nil {
		results = []Result{}
	}
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FormatResultsFrom renders a page, numbering from offset and saying when more
// remain — a caller that cannot see it withheld rows reads a truncated list as
// the whole answer.
func FormatResultsFrom(results []Result, total, offset int) string {
	if len(results) == 0 {
		return "No results found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d result(s):\n\n", total)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n", offset+i+1, r.Title)
		fmt.Fprintf(&b, "   ID: %s", r.ID)
		if r.Project != "" {
			fmt.Fprintf(&b, " | Project: %s", r.Project)
		}
		b.WriteString("\n")
		if len(r.Tags) > 0 {
			fmt.Fprintf(&b, "   Tags: %s\n", strings.Join(r.Tags, ", "))
		}
		fmt.Fprintf(&b, "   Trust: %s", r.Trust)
		if lc := r.Lifecycle(); lc != "stable" {
			fmt.Fprintf(&b, " | %s", lc)
		}
		if r.Stale {
			fmt.Fprintf(&b, " | stale since %s", r.StaleAfter)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "   File: %s\n", r.FilePath)
		if r.Snippet != "" {
			// snippetFor marks its own cuts with "...", so none are added here.
			fmt.Fprintf(&b, "   %s\n", r.Snippet)
		}
		b.WriteString("\n")
	}
	if shown := offset + len(results); shown < total {
		fmt.Fprintf(&b, "… %d more — page with --offset %d\n", total-shown, shown)
	}
	return b.String()
}
