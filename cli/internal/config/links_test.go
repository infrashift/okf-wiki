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
	content := "---\nid: \"" + id + "\"\ntitle: \"T\"\ntype: \"note\"\ncategory: \"untethered\"\ntags: []\ncreated: \"2026-02-13T10:45:00Z\"\ntimestamp: \"2026-02-13T10:45:00Z\"\n---\n\n# T\n"
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveBodyLinkIDs(t *testing.T) {
	root := t.TempDir()
	idAlpha := "019680f0-7e00-7abc-8def-1234567890ab"
	idBeta := "01968109-4700-7def-9abc-ffffffffffff"
	srcPath := writeConcept(t, root, "untethered/alpha--0001.md", idAlpha)
	writeConcept(t, root, "projects/okf/beta--0002.md", idBeta)

	body := "Absolute [Beta](/projects/okf/beta--0002.md), " +
		"relative [up](../../projects/okf/beta--0002.md), " +
		"external [site](https://example.com/x.md), " +
		"broken [gone](/nope.md)."

	got := ResolveBodyLinkIDs(body, srcPath, root)

	// Both the absolute and relative links resolve to Beta (deduplicated to one);
	// the external and broken links are skipped.
	if len(got) != 1 || got[0] != idBeta {
		t.Fatalf("expected [%s], got %v", idBeta, got)
	}
}

func TestResolveBodyLinkIDs_NoLinks(t *testing.T) {
	root := t.TempDir()
	src := writeConcept(t, root, "a--0001.md", "019680f0-7e00-7abc-8def-1234567890ab")
	if got := ResolveBodyLinkIDs("plain text, no links", src, root); got != nil {
		t.Errorf("expected nil, got %v", got)
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
