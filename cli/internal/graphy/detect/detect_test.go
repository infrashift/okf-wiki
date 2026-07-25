package detect

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

// TestListModesAgree pins the invariant behind graphy's self-ingestion bug:
// whether a repo happens to be a git checkout must never change which files a
// build sees. The walk always pruned dot-directories locally; `git ls-files`
// returns whatever the repo tracks, and okfw deliberately tracks its bundle's
// concept markdown. So before both modes shared one filter, the build after a
// `graphy export okfw` re-ingested the bundle as heading nodes.
func TestListModesAgree(t *testing.T) {
	seed := func(root string) {
		write(t, root, "main.go", "package main\n")
		write(t, root, "lib/util.py", "def f():\n    pass\n")
		write(t, root, "docs/guide.md", "# guide\n")
		// graphy's and okfw's own output, tracked by git in a real repo.
		write(t, root, ".okfw/graphy/demo/handle--0001.md", "# Handle\n")
		write(t, root, ".okfw/index.md", "# index\n")
		write(t, root, ".graphy/graph.json", "{}\n")
		// Other dot-directories that are state, not source.
		write(t, root, ".terraform/modules/vendored/main.tf", "resource \"x\" \"y\" {}\n")
		write(t, root, ".github/workflows/ci.yaml", "on: push\n")
	}

	walkRoot := t.TempDir()
	seed(walkRoot)
	walked, err := List(walkRoot)
	if err != nil {
		t.Fatalf("List (walk): %v", err)
	}

	gitRoot := t.TempDir()
	seed(gitRoot)
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "add", "-A"},
		{"-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-qm", "seed"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v (%s)", err, out)
		}
	}
	fromGit, err := List(gitRoot)
	if err != nil {
		t.Fatalf("List (git): %v", err)
	}

	walkPaths, gitPaths := paths(walked), paths(fromGit)
	if !reflect.DeepEqual(walkPaths, gitPaths) {
		t.Errorf("listing modes disagree:\n walk: %v\n  git: %v", walkPaths, gitPaths)
	}
	want := map[string]string{
		"main.go":       "go",
		"lib/util.py":   "python",
		"docs/guide.md": "markdown",
	}
	if !reflect.DeepEqual(gitPaths, want) {
		t.Errorf("git mode = %v, want %v", gitPaths, want)
	}
	// Named explicitly: these are the paths that caused the feedback loop.
	for _, banned := range []string{
		".okfw/graphy/demo/handle--0001.md",
		".okfw/index.md",
		".terraform/modules/vendored/main.tf",
	} {
		if _, ok := gitPaths[banned]; ok {
			t.Errorf("%s was ingested; graphy must not read its own output", banned)
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
