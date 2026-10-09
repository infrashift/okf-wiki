package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConcept(t *testing.T, root, rel, id string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: \"" + id + "\"\ntitle: \"T\"\ntype: \"note\"\ntags: []\ncreated: \"2026-02-13T10:45:00Z\"\ntimestamp: \"2026-02-13T10:45:00Z\"\n---\n\n# T\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveBodyLinks(t *testing.T) {
	root := t.TempDir()
	idBeta := "01968109-4700-7def-9abc-ffffffffffff"
	writeConcept(t, root, "projects/okf/beta--0002.md", idBeta)
	keyOf := func(rel string) string { return ReadConceptKey(root, rel) }

	body := "Absolute [Beta](/projects/okf/beta--0002.md), " +
		"relative [up](../../projects/okf/beta--0002.md), " +
		"external [site](https://example.com/x.md), " +
		"broken [gone](/nope.md)."

	got := ResolveBodyLinks(body, "notes/sub", keyOf)

	// Both the absolute and relative links resolve to Beta (deduplicated to one);
	// the external and broken links are skipped.
	if len(got) != 1 || got[0] != idBeta {
		t.Fatalf("expected [%s], got %v", idBeta, got)
	}
	if got := ResolveBodyLinks("plain text, no links", "", keyOf); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// A concept okfw did not write may carry no id; the spec identifies it by its
// path, and a link to it must still resolve to something.
func TestReadConceptKeyFallsBackToTheConceptID(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "metrics", "revenue.md")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("---\ntype: Metric\n---\n# Revenue\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := ReadConceptKey(root, "metrics/revenue.md"); got != "metrics/revenue" {
		t.Errorf("key = %q, want metrics/revenue", got)
	}
	if got := ReadConceptKey(root, "missing.md"); got != "" {
		t.Errorf("missing file key = %q, want empty", got)
	}
}

func TestReadConceptID(t *testing.T) {
	root := t.TempDir()
	id := "019680f0-7e00-7abc-8def-1234567890ab"
	p := writeConcept(t, root, "a--0001.md", id)
	if got := ReadConceptID(p); got != id {
		t.Errorf("ReadConceptID = %q, want %q", got, id)
	}
	if got := ReadConceptID(filepath.Join(root, "missing.md")); got != "" {
		t.Errorf("missing file should yield empty id, got %q", got)
	}
}
