package bundle_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/infrashift/okf-wiki/pkg/bundle"
)

// TestDirFSRefusesLinksOutOfTheBundle: OKF v0.3 section 3.2 — a consumer
// does not follow a symbolic link out of the bundle root, whether the link is
// the file or a directory above it. A link that stays inside works.
func TestDirFSRefusesLinksOutOfTheBundle(t *testing.T) {
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("---\ntype: Secret\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWrite := func(rel, s string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("notes/a.md", "---\ntype: Note\n---\n")
	mustWrite("notes/b.md", "---\ntype: Note\n---\n")
	for link, target := range map[string]string{
		"notes/leak.md":  secret,                               // a file out of the bundle
		"outdir":         outside,                              // a directory out of the bundle
		"notes/alias.md": filepath.Join(root, "notes", "a.md"), // a file inside
		"aliasdir":       filepath.Join(root, "notes"),         // a directory inside
	} {
		if err := os.Symlink(target, filepath.Join(root, filepath.FromSlash(link))); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	fsys := bundle.DirFS(root)
	for name, want := range map[string]bool{
		"notes/a.md":       true,
		"notes/b.md":       true,
		"notes/alias.md":   true,
		"aliasdir/a.md":    true,
		"notes/leak.md":    false,
		"outdir/secret.md": false,
	} {
		f, err := fsys.Open(name)
		if f != nil {
			f.Close()
		}
		if got := err == nil; got != want {
			t.Errorf("Open(%s): err = %v, want allowed=%v", name, err, want)
		}
		if !want && !errors.Is(err, bundle.ErrInvalidPath) {
			t.Errorf("Open(%s): err = %v, want ErrInvalidPath", name, err)
		}
	}
	// The walk never reads the leaked file's content.
	err := bundle.Walk(fsys, ".", bundle.Limits{MaxFileBytes: 1 << 20, MaxFiles: 100}, func(p string, content []byte) error {
		if string(content) == "---\ntype: Secret\n---\n" {
			t.Errorf("Walk read %s from outside the bundle", p)
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrInvalid) {
		t.Logf("Walk: %v", err)
	}
}
