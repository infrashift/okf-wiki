package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitAndDiscover(t *testing.T) {
	root := t.TempDir()
	w, err := Init(root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if fi, err := os.Stat(w.Dir()); err != nil || !fi.IsDir() {
		t.Fatalf(".graphy missing: %v", err)
	}
	if _, err := os.Stat(w.IgnorePath()); err != nil {
		t.Errorf(".graphyignore not seeded: %v", err)
	}
	gi, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil || !strings.Contains(string(gi), ".graphy/") {
		t.Errorf(".gitignore entry missing: %q %v", gi, err)
	}

	// Discover from a nested directory.
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	found, err := Discover(nested)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if found.Root != w.Root {
		t.Errorf("Discover root = %q, want %q", found.Root, w.Root)
	}
}

func TestDiscoverFailsWithoutWorkspace(t *testing.T) {
	if _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("expected ErrNoWorkspace")
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root); err != nil {
		t.Fatalf("second Init: %v", err)
	}
	gi, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Count(string(gi), ".graphy/") != 1 {
		t.Errorf("duplicate gitignore entries: %q", gi)
	}
}
