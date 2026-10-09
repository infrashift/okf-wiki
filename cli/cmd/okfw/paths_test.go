package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIndexMdRefusesDirectoriesOutsideAnyBundle: `okfw index-md <path>` used to
// treat any directory as a bundle root, prune every index.md there that no okfw
// concept justified, and delete the directories that emptied.
func TestIndexMdRefusesDirectoriesOutsideAnyBundle(t *testing.T) {
	bundleAt(t)
	elsewhere := t.TempDir()
	handWritten := filepath.Join(elsewhere, "y", "index.md")
	writeConceptFile(t, elsewhere, "y/index.md", "# Hand-written listing\n")

	if _, err := runCmd(t, rootCmd(), "index-md", elsewhere); err == nil {
		t.Error("index-md on a directory outside any bundle succeeded")
	}
	if _, err := os.Stat(handWritten); err != nil {
		t.Errorf("hand-written index.md outside the bundle is gone: %v", err)
	}
}

func TestBundleArgScopesToTheDiscoveredBundle(t *testing.T) {
	cfg := bundleAt(t)
	sub := filepath.Join(cfg.RootPath, "notes")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Dir(cfg.RootPath)

	for arg, want := range map[string]string{
		"":           cfg.RootPath, // no argument: discovered from the cwd
		project:      cfg.RootPath, // the project containing the bundle
		cfg.RootPath: cfg.RootPath, // the bundle itself
		sub:          sub,          // a subtree inside it
	} {
		var args []string
		if arg != "" {
			args = []string{arg}
		}
		got, root, err := bundleArg(args)
		if err != nil {
			t.Errorf("bundleArg(%q): %v", arg, err)
			continue
		}
		if got.RootPath != cfg.RootPath || root != want {
			t.Errorf("bundleArg(%q) = %s, %s; want %s, %s", arg, got.RootPath, root, cfg.RootPath, want)
		}
	}
	if _, _, err := bundleArg([]string{filepath.Join(cfg.RootPath, "missing")}); err == nil {
		t.Error("bundleArg accepted a directory that does not exist")
	}
}

// TestExportRefusesIDsThatEscapeTheExportDirectory: the export names each file
// after the concept's frontmatter id, which anyone with write access to the
// bundle (or a cloned repository) controls.
func TestExportRefusesIDsThatEscapeTheExportDirectory(t *testing.T) {
	cfg := bundleAt(t)
	project := filepath.Dir(cfg.RootPath)
	// Linked both ways so export, which walks one connected component, reaches
	// both whichever it starts from.
	writeConceptFile(t, cfg.RootPath, "evil--0001.md",
		conceptContent("../../escaped", "Evil", "note", "# Evil\n\n[fine](/fine--0002.md)"))
	writeConceptFile(t, cfg.RootPath, "fine--0002.md",
		conceptContent("019680f0-7e00-7abc-8def-1234567890ab", "Fine", "note", "# Fine\n\n[evil](/evil--0001.md)"))

	if _, err := runCmd(t, rootCmd(), "export"); err != nil {
		t.Fatalf("export: %v", err)
	}
	for _, p := range []string{
		filepath.Join(project, "escaped.md"),
		filepath.Join(filepath.Dir(project), "escaped.md"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("export wrote %s outside ephemeral/", p)
		}
	}
	if _, err := os.Stat(filepath.Join(cfg.RootPath, "ephemeral", "019680f0-7e00-7abc-8def-1234567890ab.md")); err != nil {
		t.Errorf("safe concept not exported: %v", err)
	}
}
