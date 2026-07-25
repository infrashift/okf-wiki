// Package workspace locates and initializes graphy's per-repo output
// directory (.graphy/), discovered by walking upward from the cwd the same
// way okfw discovers .okfw/ bundles.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DirName is graphy's output directory at the repo root.
const DirName = ".graphy"

// IgnoreFileName holds gitignore-style patterns applied on top of gitignore.
const IgnoreFileName = ".graphyignore"

// Workspace is a discovered graphy workspace.
type Workspace struct {
	Root string // repo root (parent of .graphy/)
}

// Dir returns the .graphy directory path.
func (w *Workspace) Dir() string { return filepath.Join(w.Root, DirName) }

// DBPath returns the bbolt graph database path.
func (w *Workspace) DBPath() string { return filepath.Join(w.Dir(), "graph.db") }

// JSONPath returns the portable graph.json path.
func (w *Workspace) JSONPath() string { return filepath.Join(w.Dir(), "graph.json") }

// ReportPath returns the GRAPH_REPORT.md path.
func (w *Workspace) ReportPath() string { return filepath.Join(w.Dir(), "GRAPH_REPORT.md") }

// IgnorePath returns the repo-root .graphyignore path.
func (w *Workspace) IgnorePath() string { return filepath.Join(w.Root, IgnoreFileName) }

// ErrNoWorkspace reports that no .graphy directory was found.
type ErrNoWorkspace struct{ Start string }

func (e *ErrNoWorkspace) Error() string {
	return fmt.Sprintf("no %s workspace found from %s upward (run `graphy init`)", DirName, e.Start)
}

// Discover walks upward from startDir looking for a .graphy directory.
func Discover(startDir string) (*Workspace, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, DirName)); err == nil && fi.IsDir() {
			return &Workspace{Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, &ErrNoWorkspace{Start: startDir}
		}
		dir = parent
	}
}

// Init creates .graphy/ at root, seeds an empty .graphyignore, and appends
// .graphy/ to the repo's .gitignore when one is missing the entry.
func Init(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	w := &Workspace{Root: abs}
	if err := os.MkdirAll(w.Dir(), 0755); err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", w.Dir(), err)
	}
	if _, err := os.Stat(w.IgnorePath()); os.IsNotExist(err) {
		seed := "# graphy ignore patterns (gitignore syntax), applied on top of .gitignore\n"
		if err := os.WriteFile(w.IgnorePath(), []byte(seed), 0644); err != nil {
			return nil, err
		}
	}
	if err := ensureGitignore(abs); err != nil {
		return nil, err
	}
	return w, nil
}

// ensureGitignore appends a ".graphy/" entry to root/.gitignore unless already present.
func ensureGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	entry := DirName + "/"
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == entry || trimmed == DirName {
			return nil
		}
	}
	out := string(content)
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	out += entry + "\n"
	return os.WriteFile(path, []byte(out), 0644)
}
