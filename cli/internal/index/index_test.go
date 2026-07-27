package index

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blevesearch/bleve/v2"
)

// Fixture ids, named so failures are readable.
const (
	idAuthNote  = "20260201100000-aaa" // note, stable
	idDraftSpec = "20260202100000-bbb" // note, draft
	idOldAPI    = "20260203100000-ccc" // note, deprecated + long stale
	idDaily     = "20260204100000-ddd" // daily-note, no status
	idIssue     = "20260205100000-eee" // issue, stale_after in the past
	idFuture    = "20260206100000-fff" // note, stale_after far in the future
)

// setupTestIndex builds a temporary Bleve index and returns its path.
func setupTestIndex(t *testing.T) string {
	t.Helper()
	indexPath := t.TempDir() + "/test.bleve"
	idx, err := OpenOrCreateIndex(indexPath)
	if err != nil {
		t.Fatalf("failed to create index: %v", err)
	}

	past := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	future := time.Now().AddDate(5, 0, 0).Format("2006-01-02")

	docs := []ConceptDoc{
		{
			ID: idAuthNote, Title: "Authentication Design", Type: "note", Kind: "note",
			Project: "backend", Tags: []string{"auth", "design"},
			Body: "OAuth2 implementation with JWT tokens", FilePath: "/notes/auth-design.md",
			Created: "2026-02-01T10:00:00Z", Status: "stable",
		},
		{
			ID: idDraftSpec, Title: "Payments Spec", Type: "note", Kind: "note",
			Project: "backend", Tags: []string{"payments"},
			Body: "Draft of the payments integration", FilePath: "/notes/payments.md",
			Created: "2026-02-02T10:00:00Z", Status: "draft",
		},
		{
			ID: idOldAPI, Title: "Legacy API v1", Type: "note", Kind: "note",
			Project: "docs", Tags: []string{"docs", "legacy"},
			Body: "Superseded by v2", FilePath: "/notes/legacy-api.md",
			Created: "2026-02-03T10:00:00Z", Status: "deprecated", StaleAfter: past,
		},
		{
			ID: idDaily, Title: "Daily 2026-02-04", Type: "daily-note", Kind: "daily-note",
			Tags: []string{"daily"},
			Body: "Morning standup notes and planning", FilePath: "/notes/daily-2026-02-04.md",
			Created: "2026-02-04T08:00:00Z",
		},
		{
			ID: idIssue, Title: "Security audit findings", Type: "issue", Kind: "issue",
			Project: "backend", Tags: []string{"security", "audit"},
			Body: "Found XSS vulnerability in search endpoint", FilePath: "/notes/security-audit.md",
			Created: "2026-02-05T10:00:00Z", Status: "stable", StaleAfter: past,
		},
		{
			ID: idFuture, Title: "Long-lived reference", Type: "note", Kind: "note",
			Project: "backend", Tags: []string{"reference"},
			Body: "Move to repository pattern", FilePath: "/notes/reference.md",
			Created: "2026-02-06T10:00:00Z", Status: "stable", StaleAfter: future,
		},
	}

	for i := range docs {
		if err := IndexDocument(idx, &docs[i]); err != nil {
			idx.Close()
			t.Fatalf("failed to index %s: %v", docs[i].ID, err)
		}
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}
	return indexPath
}

// resultIDs collapses hits to a set for order-independent assertions.
func resultIDs(results []SearchResult) map[string]bool {
	ids := map[string]bool{}
	for _, r := range results {
		ids[r.ID] = true
	}
	return ids
}

func search(t *testing.T, path string, opts SearchOptions) []SearchResult {
	t.Helper()
	idx, err := OpenOrCreateIndex(path)
	if err != nil {
		t.Fatalf("failed to open index: %v", err)
	}
	defer idx.Close()
	if opts.Limit == 0 {
		opts.Limit = 50
	}
	results, _, err := Search(idx, opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	return results
}

func TestSearchByType(t *testing.T) {
	path := setupTestIndex(t)

	tests := []struct {
		name     string
		typeVal  string
		wantIDs  []string
		dontWant []string
	}{
		{"note", "note", []string{idAuthNote, idDraftSpec, idOldAPI, idFuture}, []string{idDaily, idIssue}},
		{"daily-note", "daily-note", []string{idDaily}, []string{idAuthNote}},
		{"issue", "issue", []string{idIssue}, []string{idAuthNote, idDaily}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resultIDs(search(t, path, SearchOptions{Type: tt.typeVal}))
			for _, id := range tt.wantIDs {
				if !got[id] {
					t.Errorf("expected %s in results", id)
				}
			}
			for _, id := range tt.dontWant {
				if got[id] {
					t.Errorf("did not expect %s in results", id)
				}
			}
		})
	}
}

func TestSearchByLifecycleStatus(t *testing.T) {
	path := setupTestIndex(t)

	for _, tt := range []struct {
		status  string
		wantIDs []string
	}{
		{"draft", []string{idDraftSpec}},
		{"deprecated", []string{idOldAPI}},
		{"stable", []string{idAuthNote, idIssue, idFuture}},
	} {
		t.Run(tt.status, func(t *testing.T) {
			results := search(t, path, SearchOptions{Status: tt.status})
			got := resultIDs(results)
			for _, id := range tt.wantIDs {
				if !got[id] {
					t.Errorf("expected %s for status=%s", id, tt.status)
				}
			}
			if len(results) != len(tt.wantIDs) {
				t.Errorf("status=%s returned %d results, want %d", tt.status, len(results), len(tt.wantIDs))
			}
			for _, r := range results {
				if r.Status != tt.status {
					t.Errorf("%s has status %q, want %q", r.ID, r.Status, tt.status)
				}
			}
		})
	}
}

// Staleness is a date comparison against today, inclusive of the boundary day
// (OKF section 5.5), so a past stale_after is stale and a future one is not.
func TestSearchStale(t *testing.T) {
	path := setupTestIndex(t)
	got := resultIDs(search(t, path, SearchOptions{Stale: true}))

	for _, id := range []string{idOldAPI, idIssue} {
		if !got[id] {
			t.Errorf("expected %s to be stale", id)
		}
	}
	if got[idFuture] {
		t.Error("a stale_after 5 years out must not count as stale")
	}
	if got[idAuthNote] || got[idDaily] {
		t.Error("concepts without stale_after must never be stale")
	}
}

func TestSearchCombinedFilters(t *testing.T) {
	path := setupTestIndex(t)

	// project + status
	got := resultIDs(search(t, path, SearchOptions{Project: "backend", Status: "stable"}))
	for _, id := range []string{idAuthNote, idIssue, idFuture} {
		if !got[id] {
			t.Errorf("expected %s", id)
		}
	}
	if got[idDraftSpec] {
		t.Error("draft must not appear under status=stable")
	}
	if got[idOldAPI] {
		t.Error("docs project must not appear under project=backend")
	}

	// type + stale narrows to a single hit
	results := search(t, path, SearchOptions{Type: "issue", Stale: true})
	if len(results) != 1 || results[0].ID != idIssue {
		t.Errorf("type=issue + stale = %v, want just %s", resultIDs(results), idIssue)
	}

	// tags AND together
	got = resultIDs(search(t, path, SearchOptions{Tags: []string{"docs", "legacy"}}))
	if !got[idOldAPI] || len(got) != 1 {
		t.Errorf("tags AND = %v, want just %s", got, idOldAPI)
	}
}

func TestSearchFullTextWithFilters(t *testing.T) {
	path := setupTestIndex(t)

	results := search(t, path, SearchOptions{Query: "authentication", Type: "note"})
	if len(results) == 0 {
		t.Fatal("expected a hit for 'authentication' + type=note")
	}
	if results[0].ID != idAuthNote {
		t.Errorf("expected %s first, got %s", idAuthNote, results[0].ID)
	}

	// The same query filtered to a type the note does not have returns nothing.
	if got := resultIDs(search(t, path, SearchOptions{Query: "authentication", Type: "issue"})); got[idAuthNote] {
		t.Error("did not expect the note in issue-filtered results")
	}
}

func TestSearchMatchAll(t *testing.T) {
	path := setupTestIndex(t)
	idx, err := OpenOrCreateIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	results, total, err := Search(idx, SearchOptions{Limit: 50})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if total != 6 || len(results) != 6 {
		t.Errorf("total=%d len=%d, want 6/6", total, len(results))
	}
}

func TestSearchLimitAndOffset(t *testing.T) {
	path := setupTestIndex(t)
	first := search(t, path, SearchOptions{Limit: 2})
	if len(first) != 2 {
		t.Fatalf("limit=2 returned %d", len(first))
	}
	rest := search(t, path, SearchOptions{Limit: 2, Offset: 2})
	if len(rest) != 2 {
		t.Fatalf("offset page returned %d", len(rest))
	}
	for _, r := range rest {
		if resultIDs(first)[r.ID] {
			t.Errorf("%s appeared on both pages", r.ID)
		}
	}
}

// --- AllDocIDs / DeleteDocument: the pruning primitives ---

func TestAllDocIDs(t *testing.T) {
	path := setupTestIndex(t)
	idx, err := OpenOrCreateIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	ids, err := AllDocIDs(idx)
	if err != nil {
		t.Fatalf("AllDocIDs: %v", err)
	}
	if len(ids) != 6 {
		t.Errorf("got %d ids, want 6: %v", len(ids), ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, want := range []string{idAuthNote, idDraftSpec, idOldAPI, idDaily, idIssue, idFuture} {
		if !seen[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestAllDocIDsOnEmptyIndex(t *testing.T) {
	idx, err := OpenOrCreateIndex(t.TempDir() + "/empty.bleve")
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()

	ids, err := AllDocIDs(idx)
	if err != nil {
		t.Fatalf("AllDocIDs on an empty index: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("got %v, want none", ids)
	}
}

func TestDeleteDocumentRemovesItFromResults(t *testing.T) {
	path := setupTestIndex(t)
	idx, err := OpenOrCreateIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteDocument(idx, idAuthNote); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatal(err)
	}

	if got := resultIDs(search(t, path, SearchOptions{})); got[idAuthNote] {
		t.Error("deleted document still appears in search results")
	}
}

// --- rendering layer ---

func TestFormatSearchResultsEmpty(t *testing.T) {
	if got := FormatSearchResults(nil, 0); got != "No results found." {
		t.Errorf("got %q", got)
	}
}

func TestFormatSearchResults(t *testing.T) {
	out := FormatSearchResults([]SearchResult{{
		ID: idAuthNote, Title: "Authentication Design", Project: "backend",
		Tags: []string{"auth", "design"}, FilePath: "/notes/auth-design.md",
		Snippet: "OAuth2 tokens", Status: "stable",
	}}, 1)

	for _, want := range []string{
		"Found 1 results:",
		"1. Authentication Design",
		"ID: " + idAuthNote,
		"Project: backend",
		"Tags: auth, design",
		"File: /notes/auth-design.md",
		"...OAuth2 tokens...",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// "stable" is the default, so it is not worth a line of terminal output.
	if strings.Contains(out, "Status:") {
		t.Errorf("stable status should not be printed:\n%s", out)
	}
}

func TestFormatSearchResultsFlagsDraftDeprecatedAndStale(t *testing.T) {
	past := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	out := FormatSearchResults([]SearchResult{
		{ID: "a", Title: "Draft thing", Status: "draft"},
		{ID: "b", Title: "Old thing", Status: "deprecated", StaleAfter: past},
	}, 2)

	for _, want := range []string{"Status: draft", "Status: deprecated", "STALE since " + past} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestSearchResultsToJSON(t *testing.T) {
	out, err := SearchResultsToJSON([]SearchResult{{ID: "a", Title: "T", Status: "draft"}})
	if err != nil {
		t.Fatalf("SearchResultsToJSON: %v", err)
	}
	var back []SearchResult
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(back) != 1 || back[0].ID != "a" || back[0].Status != "draft" {
		t.Errorf("round-trip lost data: %+v", back)
	}
	// Empty optional fields stay out of the payload; agents read this.
	if strings.Contains(out, "stale_after") {
		t.Errorf("empty stale_after should be omitted:\n%s", out)
	}
}

func TestSearchResultIsStale(t *testing.T) {
	today := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name       string
		staleAfter string
		want       bool
	}{
		{"absent", "", false},
		{"yesterday", "2026-07-24", true},
		{"today is inclusive", "2026-07-25", true},
		{"tomorrow", "2026-07-26", false},
		{"unparseable", "not-a-date", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := SearchResult{StaleAfter: tt.staleAfter}
			if got := r.IsStale(today); got != tt.want {
				t.Errorf("IsStale(%q) = %v, want %v", tt.staleAfter, got, tt.want)
			}
		})
	}
}

// TestSearchSnippetsCarryNoMarkupAndAreDeterministic covers two defects that
// shared one line. Bleve's default "html" highlighter runs html.EscapeString
// over the text *surrounding* each match, so a snippet of a markdown concept
// containing & or < came back corrupted, not merely tagged — stripping <mark>
// in Go would not have fixed it. And the field was chosen by ranging
// hit.Fragments, a Go map, and breaking on the first entry.
func TestSearchSnippetsCarryNoMarkupAndAreDeterministic(t *testing.T) {
	idx, cleanup := freshIndex(t)
	defer cleanup()
	doc := &ConceptDoc{
		ID:       "019680f0-7e00-7abc-8def-100000000000",
		Title:    "Escaping",
		Type:     "note",
		Kind:     "note",
		Body:     "The parser handles a & b and <html> tags when tokenizing input.",
		FilePath: "/w/escaping.md",
	}
	if err := IndexDocument(idx, doc); err != nil {
		t.Fatal(err)
	}

	var first string
	for i := 0; i < 20; i++ {
		results, _, err := Search(idx, SearchOptions{Query: "tokenizing"})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 {
			t.Fatalf("results = %d", len(results))
		}
		got := results[0].Snippet
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("snippet differs between identical searches:\n%q\n%q", first, got)
		}
	}
	for _, bad := range []string{"<mark>", "&amp;", "&lt;", "<start>"} {
		if strings.Contains(first, bad) {
			t.Errorf("snippet contains %s: %q", bad, first)
		}
	}
	if !strings.Contains(first, "a & b") {
		t.Errorf("snippet lost the literal text: %q", first)
	}
}

// TestSearchPrefersTheDescriptionOverAFragment: the OKF one-sentence summary
// exists to say what a concept is, was indexed, and was never returned.
func TestSearchPrefersTheDescriptionOverAFragment(t *testing.T) {
	idx, cleanup := freshIndex(t)
	defer cleanup()
	if err := IndexDocument(idx, &ConceptDoc{
		ID: "019680f0-7e00-7abc-8def-100000000001", Title: "Auth", Type: "note", Kind: "note",
		Description: "How the session token is minted, signed and rotated.",
		Body:        "a long body mentioning tokens somewhere in the middle of it",
		FilePath:    "/w/auth.md",
	}); err != nil {
		t.Fatal(err)
	}
	results, _, err := Search(idx, SearchOptions{Query: "tokens"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
	if results[0].Description == "" {
		t.Error("description was not returned")
	}
	if results[0].Snippet != results[0].Description {
		t.Errorf("snippet = %q, want the description", results[0].Snippet)
	}
}

// TestFormatSearchResultsAnnouncesWithheldRows: it printed the true total and
// then listed only the capped slice, so a 57-hit search read as if all 57 were
// on screen.
func TestFormatSearchResultsAnnouncesWithheldRows(t *testing.T) {
	results := []SearchResult{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}

	out := FormatSearchResults(results, 57)
	if !strings.Contains(out, "55 more") {
		t.Errorf("withheld rows not announced:\n%s", out)
	}
	if !strings.Contains(out, "--offset 2") {
		t.Errorf("continuation offset not given:\n%s", out)
	}

	// A paged call must suggest the right next offset, not restart from 0.
	if out := FormatSearchResultsFrom(results, 57, 20); !strings.Contains(out, "--offset 22") {
		t.Errorf("paged continuation offset is wrong:\n%s", out)
	}
	// And a complete page says nothing.
	if out := FormatSearchResults(results, 2); strings.Contains(out, "more") {
		t.Errorf("a complete page claimed truncation:\n%s", out)
	}
}


// freshIndex is an empty index for tests that assert on a single document.
func freshIndex(t *testing.T) (bleve.Index, func()) {
	t.Helper()
	idx, err := OpenOrCreateIndex(filepath.Join(t.TempDir(), "idx"))
	if err != nil {
		t.Fatal(err)
	}
	return idx, func() { idx.Close() }
}
