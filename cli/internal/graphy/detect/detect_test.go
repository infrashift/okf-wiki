package detect

import (
	"os"
	"path/filepath"
	"testing"
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

func paths(files []File) map[string]string {
	m := map[string]string{}
	for _, f := range files {
		m[f.Path] = f.Language
	}
	return m
}

func TestListWalkFallback(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n")
	write(t, root, "lib/util.py", "def f():\n    pass\n")
	write(t, root, "web/app.tsx", "export const A = 1\n")
	write(t, root, "README.md", "# hi\n")
	write(t, root, "notes.txt", "not a source file\n")
	write(t, root, "node_modules/dep/index.js", "module.exports = 1\n")
	write(t, root, ".hidden/secret.go", "package secret\n")

	files, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := paths(files)

	want := map[string]string{
		"main.go":     "go",
		"lib/util.py": "python",
		"web/app.tsx": "typescript",
		"README.md":   "markdown",
	}
	for p, lang := range want {
		if got[p] != lang {
			t.Errorf("%s: language = %q, want %q (all: %v)", p, got[p], lang, got)
		}
	}
	for _, banned := range []string{"notes.txt", "node_modules/dep/index.js", ".hidden/secret.go"} {
		if _, ok := got[banned]; ok {
			t.Errorf("%s should have been excluded", banned)
		}
	}
	for _, f := range files {
		if f.SHA256 == "" || f.Size == 0 {
			t.Errorf("%s: missing hash/size", f.Path)
		}
	}
}

func TestGraphyignore(t *testing.T) {
	root := t.TempDir()
	write(t, root, "main.go", "package main\n")
	write(t, root, "gen/generated.go", "package gen\n")
	write(t, root, "big_test.go", "package main\n")
	write(t, root, ".graphyignore", "# comment\ngen/\n*_test.go\n")

	files, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := paths(files)
	if _, ok := got["main.go"]; !ok {
		t.Errorf("main.go missing: %v", got)
	}
	if _, ok := got["gen/generated.go"]; ok {
		t.Errorf("gen/ not ignored")
	}
	if _, ok := got["big_test.go"]; ok {
		t.Errorf("*_test.go not ignored")
	}
}

func TestBinarySniff(t *testing.T) {
	root := t.TempDir()
	write(t, root, "fake.go", "package main\x00\xff binary")
	files, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("binary file not excluded: %v", paths(files))
	}
}
