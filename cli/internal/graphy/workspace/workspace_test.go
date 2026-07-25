package workspace

import (
	"errors"
	"io"
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

func TestWriteFileAtomicReplacesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "graph.json")
	if err := os.WriteFile(path, []byte("previous good content"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteFileAtomic(path, func(w io.Writer) error {
		_, err := io.WriteString(w, "new content")
		return err
	}); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content" {
		t.Errorf("content = %q, want %q", got, "new content")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0644 {
		t.Errorf("mode = %o, want 644 (CreateTemp makes it 0600)", perm)
	}
	assertNoTempLeftBehind(t, dir)
}

// TestWriteFileAtomicPreservesPreviousOnFailure is the reason the helper
// exists: the writers it replaced called os.Create first, truncating the last
// good file before knowing whether a new one could be produced.
func TestWriteFileAtomicPreservesPreviousOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "GRAPH_REPORT.md")
	if err := os.WriteFile(path, []byte("previous good report"), 0644); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("render exploded")
	err := WriteFileAtomic(path, func(w io.Writer) error {
		io.WriteString(w, "half a report") // partial write before failing
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous good report" {
		t.Errorf("a failed write damaged the previous file: %q", got)
	}
	assertNoTempLeftBehind(t, dir)
}

func TestWriteFileAtomicCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.json")
	if err := WriteFileAtomic(path, func(w io.Writer) error {
		_, err := io.WriteString(w, "{}")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "{}" {
		t.Errorf("content = %q, %v", got, err)
	}
	assertNoTempLeftBehind(t, dir)
}

func TestWriteFileAtomicErrorsOnMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope", "out.json")
	if err := WriteFileAtomic(path, func(w io.Writer) error { return nil }); err == nil {
		t.Error("writing into a missing directory should fail")
	}
}

// assertNoTempLeftBehind checks the helper cleans up after itself; a stray
// .tmp file in .graphy/ would be picked up by the next directory listing.
func assertNoTempLeftBehind(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}
