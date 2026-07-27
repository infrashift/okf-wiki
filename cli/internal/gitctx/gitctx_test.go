package gitctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// gitInit makes dir a repository. Returns the path git itself reports as the
// toplevel, which may differ from dir on systems where the temp dir is a
// symlink (macOS /var -> /private/var).
func gitInit(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return string(out[:len(out)-1])
}

func TestGetGitRootFromRepoRoot(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	want := gitInit(t, dir)

	if got := GetGitRoot(dir); got != want {
		t.Errorf("GetGitRoot(%q) = %q, want %q", dir, got, want)
	}
}

// `okfw init` runs from wherever the user happens to be, so the walk upward is
// the behaviour that matters.
func TestGetGitRootFromNestedSubdirectory(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	want := gitInit(t, dir)

	nested := filepath.Join(dir, "cli", "cmd", "okfw")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if got := GetGitRoot(nested); got != want {
		t.Errorf("GetGitRoot(%q) = %q, want %q", nested, got, want)
	}
}

// Outside a repository the caller falls back to the current directory, which
// depends on this returning empty rather than erroring.
func TestGetGitRootOutsideARepoIsEmpty(t *testing.T) {
	requireGit(t)
	// t.TempDir is under /tmp, which is not itself a repository. Guard against
	// a machine where it somehow is, rather than reporting a confusing failure.
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		t.Skipf("temp dir is inside a git repo (%s); cannot test the outside case", out)
	}
	if got := GetGitRoot(dir); got != "" {
		t.Errorf("GetGitRoot outside a repo = %q, want empty", got)
	}
}

func TestGetGitRootWithNonexistentDir(t *testing.T) {
	requireGit(t)
	missing := filepath.Join(t.TempDir(), "does", "not", "exist")
	if got := GetGitRoot(missing); got != "" {
		t.Errorf("GetGitRoot on a missing dir = %q, want empty", got)
	}
}

// The returned path must be usable directly as a parent for .okfw/.
func TestGetGitRootIsAbsolute(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitInit(t, dir)

	got := GetGitRoot(dir)
	if got == "" {
		t.Fatal("expected a root")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("GetGitRoot = %q, want an absolute path", got)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Errorf("GetGitRoot = %q, which is not a directory: %v", got, err)
	}
}
