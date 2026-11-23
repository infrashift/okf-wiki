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
	"github.com/infrashift/okf-wiki/internal/graph"
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

	Status string // OKF lifecycle: draft/stable/deprecated, or a raw unknown value
	Stale  bool   // only concepts whose stale_after has passed

	// Current keeps only current knowledge (OKF v0.3 section 5.4): no
	// deprecated concept and none with a status outside the vocabulary. A
	// deprecated hit with superseded_by is replaced by the concept it names,
	// as the section asks of a consumer that filters to current knowledge.
	Current bool
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

	Status       string `json:"status,omitempty"`
	StaleAfter   string `json:"stale_after,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`

	// Replaces names the deprecated concept a --current search followed to
	// this hit through its superseded_by.
	Replaces string `json:"replaces,omitempty"`

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
	// Every concept seen, matching or not, so a --current search can hand
	// back the replacement a deprecated hit's superseded_by names.
	byRel := make(map[string]Result)
	err = okfbundle.Walk(okfbundle.DirFS(bundleRoot), dir, unbounded, func(rel string, content []byte) error {
		fm, err := config.ParseFrontmatter(content)
		if err != nil || !config.IsConcept(fm) {
			return nil // no frontmatter or no type: not a concept (OKF section 3.2)
		}
		abs, _ := filepath.Abs(filepath.Join(bundleRoot, filepath.FromSlash(rel)))
		r := Result{
			// A concept okfw did not write may carry no id; its OKF concept ID
			// (the path) is what identifies it, and what read and resolve accept.
			ID:           config.ConceptKey(fm, rel),
			Title:        fm.Title,
			Type:         fm.Type,
			Kind:         fm.GetKind(),
			Description:  fm.Description,
			Project:      fm.Project,
			Tags:         fm.Tags,
			FilePath:     abs,
			Status:       fm.Status,
			StaleAfter:   fm.StaleAfter,
			SupersededBy: fm.SupersededBy,
			Trust:        fm.TrustTier(),
			Stale:        config.IsStale(fm.StaleAfter, now),
		}
		if opts.Current {
			byRel[rel] = r
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
	results := make([]Result, 0, len(hits))
	for _, h := range hits {
		r := byConcept[h.Concept]
		r.Score, r.Snippet = h.Score, h.Snippet
		results = append(results, r)
	}
	if opts.Current {
		results = followSuperseded(bundleRoot, results, byRel, opts)
	}
	total := len(results)
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	if opts.Offset >= total {
		return nil, total, nil
	}
	end := min(opts.Offset+limit, total)
	return results[opts.Offset:end], total, nil
}

// followSuperseded applies the current-knowledge filter after ranking: a
// current hit stays; a deprecated one is replaced, at its rank, by the
// concept its superseded_by names when that concept is itself current and
// passes the other filters; anything else is dropped. A chain of
// replacements is followed a few steps, never in a loop.
func followSuperseded(bundleRoot string, hits []Result, byRel map[string]Result, opts Options) []Result {
	exists := legacyExists(bundleRoot)
	seen := make(map[string]bool, len(hits))
	for _, r := range hits {
		if isCurrent(r) {
			seen[r.ID] = true
		}
	}
	out := make([]Result, 0, len(hits))
	for _, r := range hits {
		if isCurrent(r) {
			out = append(out, r)
			continue
		}
		cur, replaced := r, false
		for step := 0; step < 8 && cur.Lifecycle() == "deprecated" && cur.SupersededBy != ""; step++ {
			rel := relOf(bundleRoot, cur.FilePath)
			target := graph.ResolveRef(cur.SupersededBy, path.Dir(rel), exists)
			next, ok := byRel[target]
			if !ok || next.ID == cur.ID {
				break
			}
			cur, replaced = next, true
		}
		if !replaced || !isCurrent(cur) || seen[cur.ID] || !matches(cur, Options{Project: opts.Project, Type: opts.Type, Kind: opts.Kind, Tags: opts.Tags, Status: opts.Status, Stale: opts.Stale}) {
			continue
		}
		seen[cur.ID] = true
		cur.Score, cur.Snippet, cur.Replaces = r.Score, "", r.ID
		out = append(out, cur)
	}
	return out
}

func isCurrent(r Result) bool {
	s := r.Lifecycle()
	return s == "draft" || s == "stable"
}

func relOf(bundleRoot, abs string) string {
	rel, err := filepath.Rel(bundleRoot, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// legacyExists is the section 6.2 fallback's existence check for a bundle
// declaring okf_version 0.2 or earlier, or none (see bundle.RefExists).
func legacyExists(bundleRoot string) func(string) bool {
	if !config.LegacyPathFallback(config.ReadDeclaredOKFVersion(bundleRoot)) {
		return nil
	}
	return func(rel string) bool {
		_, err := os.Stat(filepath.Join(bundleRoot, filepath.FromSlash(rel)))
		return err == nil
	}
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
	// OKF section 5.4: an absent status means stable, and an unknown one is
	// kept raw — it is not stable.
	if s = strings.TrimSpace(s); s == "" {
		return "stable"
	}
	return s
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

// Hit is a search result as okfw hands it to an agent: `okfw search --json`
// and the MCP search tool emit exactly this, from this one type, so the two
// surfaces cannot drift apart.
//
// It is shaped for the reader who pays per token. The CLI used to print the
// whole Result indented: an absolute file path, a score, the description beside
// the snippet, and kind "note", project "" and stale false on every hit. That
// cost twice what the MCP tool returned for the same search (2,923 against
// 1,432 tokens a query, counted with count_tokens on the okfw-benchmark query
// set), and agents following okfw's skill use the CLI form.
type Hit struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Path  string `json:"path"` // bundle-relative; okfw accepts it wherever it takes a concept
	// Kind and Type say what a hit *is*, so a caller can route on it without
	// reading the concept.
	Kind    string   `json:"kind,omitempty"`
	Type    string   `json:"type,omitempty"`
	Project string   `json:"project,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Snippet string   `json:"snippet,omitempty"`
	// Trust is always present: an agent cannot tell an omitted tier from
	// "unverified". Status and stale appear only when they are the exception,
	// so a current, stable hit costs nothing extra.
	Trust  string `json:"trust"`
	Status string `json:"status,omitempty"` // anything but stable, raw; absent means stable
	Stale  bool   `json:"stale,omitempty"`

	// SupersededBy and Replaces carry OKF v0.3 section 5.4's replacement link:
	// on a deprecated hit, and on the hit a current search gave in its place.
	SupersededBy string `json:"superseded_by,omitempty"`
	Replaces     string `json:"replaces,omitempty"`
}

// Hit renders r for an agent; bundleRoot makes its path bundle-relative.
func (r Result) Hit(bundleRoot string) Hit {
	h := Hit{
		ID:       r.ID,
		Title:    r.Title,
		Path:     relTo(bundleRoot, r.FilePath),
		Kind:     r.Kind,
		Type:     r.Type,
		Project:  r.Project,
		Tags:     r.Tags,
		Snippet:  r.Snippet,
		Trust:    r.Trust,
		Stale:    r.Stale,
		Replaces: r.Replaces,
	}
	if lc := r.Lifecycle(); lc != "stable" {
		if lc == "deprecated" {
			h.SupersededBy = r.SupersededBy
		}
		h.Status = lc
	}
	return h
}

// HitsToJSON renders results as a compact JSON array of Hits, the form
// `okfw search --json` prints.
func HitsToJSON(results []Result, bundleRoot string) (string, error) {
	hits := make([]Hit, 0, len(results))
	for _, r := range results {
		hits = append(hits, r.Hit(bundleRoot))
	}
	b, err := json.Marshal(hits)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func relTo(root, abs string) string {
	if rel, err := filepath.Rel(root, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, "../") {
		return filepath.ToSlash(rel)
	}
	return abs
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
