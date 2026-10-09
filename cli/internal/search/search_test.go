package search

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func concept(id, title, desc, body string, extra string) string {
	return "---\nid: \"" + id + "\"\ntitle: \"" + title + "\"\ntype: \"note\"\n" +
		"description: \"" + desc + "\"\ntags: []\n" +
		"created: \"2026-01-01T00:00:00Z\"\ntimestamp: \"2026-01-01T00:00:00Z\"\n" +
		extra + "---\n" + body
}

func bundleWith(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "a--0001.md", concept("019680f0-7e00-7abc-8def-100000000000",
		"Policy evaluation", "How a policy is evaluated", "The engine evaluates a policy against input.\n", ""))
	write(t, root, "b--0002.md", concept("019680f0-7e00-7abc-8def-200000000000",
		"Storage backends", "Filesystem and S3", "Policy files can live in S3. Policy policy policy.\n", ""))
	write(t, root, "index.md", "# listing\n")
	return root
}

// TestRankingPrefersTitleOverBody is the property the index was kept for: with
// a small result cap the caller needs the best hits, not the first in
// directory order. "Policy" is in b's body far more often than in a's, but a
// has it in the title.
func TestRankingPrefersTitleOverBody(t *testing.T) {
	root := bundleWith(t)
	hits, total, err := Search(root, Options{Query: "policy"})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if hits[0].Title != "Policy evaluation" {
		t.Errorf("top hit = %q, want the title match", hits[0].Title)
	}
	if hits[0].Score <= hits[1].Score {
		t.Errorf("scores not ordered: %v <= %v", hits[0].Score, hits[1].Score)
	}
}

// TestAllTermsBeatsOneTermRepeated: coverage of the query outranks repetition,
// which is the behaviour BM25 saturation buys.
func TestAllTermsBeatsOneTermRepeated(t *testing.T) {
	root := t.TempDir()
	write(t, root, "both--0001.md", concept("019680f0-7e00-7abc-8def-100000000000",
		"Alpha", "", "alpha and beta appear here\n", ""))
	write(t, root, "one--0002.md", concept("019680f0-7e00-7abc-8def-200000000000",
		"Gamma", "", "beta beta beta beta beta beta\n", ""))
	hits, _, err := Search(root, Options{Query: "alpha beta"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 || hits[0].Title != "Alpha" {
		t.Fatalf("got %v, want the concept matching both terms first", hits)
	}
}

// TestDeletedConceptIsGoneImmediately is why the index was removed: a stale
// entry used to survive until something pruned it.
func TestDeletedConceptIsGoneImmediately(t *testing.T) {
	root := bundleWith(t)
	if _, total, _ := Search(root, Options{Query: "storage"}); total != 1 {
		t.Fatalf("setup: want 1 hit, got %d", total)
	}
	if err := os.Remove(filepath.Join(root, "b--0002.md")); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := Search(root, Options{Query: "storage"}); total != 0 {
		t.Errorf("deleted concept still findable")
	}
}

func TestFiltersAndPaging(t *testing.T) {
	root := t.TempDir()
	write(t, root, "n--0001.md", concept("019680f0-7e00-7abc-8def-100000000000",
		"Note one", "", "body\n", "project: \"p1\"\n"))
	write(t, root, "n--0002.md", concept("019680f0-7e00-7abc-8def-200000000000",
		"Note two", "", "body\n", "project: \"p2\"\n"))

	if _, total, _ := Search(root, Options{Project: "p1"}); total != 1 {
		t.Errorf("project filter: total = %d, want 1", total)
	}
	// An empty query with filters must still return concepts.
	hits, total, err := Search(root, Options{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(hits) != 1 {
		t.Errorf("limit: total=%d len=%d, want 2 and 1", total, len(hits))
	}
	page2, _, _ := Search(root, Options{Limit: 1, Offset: 1})
	if len(page2) != 1 || page2[0].ID == hits[0].ID {
		t.Errorf("offset returned the same concept")
	}
}

// TestStaleFilterReadsInstants: the filter used to parse only bare dates, so a
// spec-form stale_after was never reported stale, and it compared local time
// against UTC midnight, which was off by a day east of UTC.
func TestStaleFilterReadsInstants(t *testing.T) {
	root := t.TempDir()
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	for name, stale := range map[string]string{"past": past, "future": future, "dateonly": "2000-01-01"} {
		write(t, root, name+".md", "---\nid: \"019680f0-7e00-7abc-8def-10000000000"+name[:1]+"\"\n"+
			"title: \""+name+"\"\ntype: \"note\"\ntags: []\nstale_after: \""+stale+"\"\n"+
			"created: \"2026-01-01T00:00:00Z\"\ntimestamp: \"2026-01-01T00:00:00Z\"\n---\nbody\n")
	}
	hits, _, err := Search(root, Options{Stale: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Title != "past" {
		var titles []string
		for _, h := range hits {
			titles = append(titles, h.Title)
		}
		t.Errorf("stale hits = %v, want [past] (a bare date is not a valid instant)", titles)
	}
}
