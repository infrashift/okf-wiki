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
// match rather than BM25 — see scoreConcept.
//
// The cost is linear in bundle size. At tens of thousands of concepts this is
// the design choice to revisit.
package search

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

// Options selects and filters concepts. The zero value matches everything.
type Options struct {
	Query   string   // free-text query; empty means "filters only"
	Project string   // exact project match
	Type    string   // OKF free-form type
	Kind    string   // structured kind: note/daily-note/issue
	Tags    []string // all must be present
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
}

// IsStale reports whether the concept's stale_after date has arrived.
// Inclusive: stale means today >= stale_after.
func (r Result) IsStale(today time.Time) bool {
	if r.StaleAfter == "" {
		return false
	}
	d, err := time.Parse("2006-01-02", r.StaleAfter)
	if err != nil {
		return false
	}
	return !today.Before(d)
}

// Field weights. Title outranks description outranks body because a concept
// whose *title* is the query is almost always the one meant, and okfw's own
// convention is one idea per concept with a one-sentence description.
const (
	weightTitle       = 10.0
	weightTag         = 6.0
	weightDescription = 4.0
	weightBody        = 1.0
	// bonusExactTitle fires when the whole query is the title, which is how an
	// agent following a name from the code graph arrives here.
	bonusExactTitle = 25.0
	// bonusAllTerms rewards a concept matching every term over one matching a
	// single term many times — the property BM25 saturation buys, obtained here
	// by rewarding coverage rather than by damping frequency.
	bonusAllTerms = 8.0
)

// Search walks the bundle, keeps concepts passing every filter, scores them
// against the query and returns one page plus the total number of matches.
func Search(bundleRoot string, opts Options) ([]Result, int, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	terms := tokenize(opts.Query)
	today := time.Now()

	var all []Result
	err := filepath.Walk(bundleRoot, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if bundle.SkipWalkDir(bundleRoot, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(fi.Name()) {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil // unreadable: skip, do not fail the whole search
		}
		fm, err := config.ParseFrontmatter(content)
		if err != nil || fm.ID == "" {
			return nil // not a concept
		}
		abs, _ := filepath.Abs(p)
		r := Result{
			ID:          fm.ID,
			Title:       fm.Title,
			Type:        fm.Type,
			Kind:        fm.GetKind(),
			Description: fm.Description,
			Project:     fm.Project,
			Tags:        fm.Tags,
			FilePath:    abs,
			Status:      fm.Status,
			StaleAfter:  fm.StaleAfter,
		}
		if !matches(r, opts, today) {
			return nil
		}
		body := config.ExtractBody(content)
		if len(terms) > 0 {
			score, snippet := scoreConcept(r, body, terms)
			if score <= 0 {
				return nil
			}
			r.Score, r.Snippet = score, snippet
		} else if r.Description != "" {
			r.Snippet = firstLine(r.Description)
		}
		all = append(all, r)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	// Highest score first; ties by title then id so paging is stable and two
	// runs over an unchanged bundle agree.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		if all[i].Title != all[j].Title {
			return all[i].Title < all[j].Title
		}
		return all[i].ID < all[j].ID
	})

	total := len(all)
	if opts.Offset >= total {
		return nil, total, nil
	}
	end := min(opts.Offset+limit, total)
	return all[opts.Offset:end], total, nil
}

// matches applies every non-query filter.
func lifecycleOf(s string) string {
	// OKF section 5.4: an absent status means stable.
	switch s {
	case "draft", "stable", "deprecated":
		return s
	}
	return "stable"
}

func matches(r Result, opts Options, today time.Time) bool {
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
	if opts.Stale && !r.IsStale(today) {
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

// scoreConcept ranks a concept against the query terms and returns a snippet
// around the best body match.
//
// A weighted field match rather than BM25. With one idea per concept and a
// one-sentence description, term frequency within a concept carries far less
// signal than *where* a term appears, and coverage of the query matters more
// than repetition — which is what bonusAllTerms encodes.
func scoreConcept(r Result, body string, terms []string) (float64, string) {
	title := strings.ToLower(r.Title)
	desc := strings.ToLower(r.Description)
	lowBody := strings.ToLower(body)
	tags := strings.ToLower(strings.Join(r.Tags, " "))

	var score float64
	hits := 0
	for _, t := range terms {
		var termScore float64
		if strings.Contains(title, t) {
			termScore += weightTitle
		}
		if strings.Contains(tags, t) {
			termScore += weightTag
		}
		if strings.Contains(desc, t) {
			termScore += weightDescription
		}
		if n := strings.Count(lowBody, t); n > 0 {
			// Damp repetition: a term appearing 50 times is not 50x the signal.
			termScore += weightBody * (1 + float64(min(n, 5))/2)
		}
		if termScore > 0 {
			hits++
		}
		score += termScore
	}
	if score == 0 {
		return 0, ""
	}
	if hits == len(terms) {
		score += bonusAllTerms
	}
	if title == strings.Join(terms, " ") {
		score += bonusExactTitle
	}
	return score, snippetFor(body, terms, r.Description)
}

// snippetFor returns a short window of body text around the first matching
// term, falling back to the description.
//
// Plain text, never marked up: the previous implementation used bleve's default
// html highlighter, whose formatter ran html.EscapeString over the text
// *around* each match, so a snippet of a markdown concept came back containing
// `&amp;` and `&lt;`. That was corruption, not markup.
func snippetFor(body string, terms []string, fallback string) string {
	const window = 160
	low := strings.ToLower(body)
	idx := -1
	for _, t := range terms {
		if i := strings.Index(low, t); i >= 0 && (idx < 0 || i < idx) {
			idx = i
		}
	}
	if idx < 0 {
		return firstLine(fallback)
	}
	start := max(idx-window/2, 0)
	end := min(start+window, len(body))
	s := strings.TrimSpace(strings.ReplaceAll(body[start:end], "\n", " "))
	if start > 0 {
		s = "..." + s
	}
	if end < len(body) {
		s += "..."
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// tokenize lowercases and splits a query on whitespace, dropping punctuation
// that would never match inside a word.
func tokenize(q string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(q)) {
		f = strings.Trim(f, `.,;:!?"'()[]{}`)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
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

// FormatResults renders results for a terminal.
func FormatResults(results []Result, total int) string {
	return FormatResultsFrom(results, total, 0)
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
		fmt.Fprintf(&b, "   File: %s\n", r.FilePath)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   ...%s...\n", r.Snippet)
		}
		b.WriteString("\n")
	}
	if shown := offset + len(results); shown < total {
		fmt.Fprintf(&b, "… %d more — page with --offset %d\n", total-shown, shown)
	}
	return b.String()
}
