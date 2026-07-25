// Package detect discovers the source files a build should extract.
//
// Inside a git repository it delegates file listing to
// `git ls-files --cached --others --exclude-standard` for exact gitignore
// semantics; outside git it falls back to a filesystem walk with built-in
// ignores. A .graphyignore file (gitignore-style basics) applies in both modes.
package detect

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"okfw/cli/internal/graphy/workspace"
)

// File is a discovered source file.
type File struct {
	Path     string // repo-relative, slash-separated
	AbsPath  string
	Language string // "" when not a language graphy extracts
	Size     int64
	SHA256   string
}

// langByExt maps file extensions to graphy language names.
var langByExt = map[string]string{
	".go":  "go",
	".py":  "python",
	".js":  "javascript",
	".jsx": "javascript",
	".mjs": "javascript",
	".cjs": "javascript",
	".ts":   "typescript",
	".tsx":  "typescript",
	".md":   "markdown",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".cue":  "cue",
	".tf":   "terraform",
	".tfvars": "terraform",
}

// walkSkipDirs are pruned in the non-git fallback walk.
var walkSkipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
}

// Language returns the graphy language for a path ("" when unsupported).
func Language(path string) string {
	return langByExt[strings.ToLower(filepath.Ext(path))]
}

// List discovers, filters, and hashes the extractable files under root.
// Only files with a supported language are returned, sorted by git/walk order.
func List(root string) ([]File, error) {
	rels, err := listPaths(root)
	if err != nil {
		return nil, err
	}
	ignore, err := loadIgnore(filepath.Join(root, workspace.IgnoreFileName))
	if err != nil {
		return nil, err
	}

	var files []File
	for _, rel := range rels {
		lang := Language(rel)
		if lang == "" || ignore.Match(rel) {
			continue
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		fi, err := os.Stat(abs)
		if err != nil || fi.IsDir() {
			continue // listed but vanished, or a submodule
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		if bytes.IndexByte(content, 0) >= 0 {
			continue // binary masquerading under a source extension
		}
		sum := sha256.Sum256(content)
		files = append(files, File{
			Path:     rel,
			AbsPath:  abs,
			Language: lang,
			Size:     fi.Size(),
			SHA256:   hex.EncodeToString(sum[:]),
		})
	}
	return files, nil
}

// listPaths returns candidate repo-relative paths via git when available,
// else a pruned filesystem walk.
func listPaths(root string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		if paths, err := gitListFiles(root); err == nil {
			return paths, nil
		}
	}
	return walkFiles(root)
}

func gitListFiles(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w", err)
	}
	var paths []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			paths = append(paths, line)
		}
	}
	return paths, sc.Err()
}

func walkFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := fi.Name()
		if fi.IsDir() {
			if p != root && (strings.HasPrefix(name, ".") || walkSkipDirs[name]) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	return paths, err
}

// ignoreSet is a minimal gitignore-style matcher: blank lines and #comments
// skipped; a trailing "/" matches a directory prefix; leading "/" anchors to
// the root; otherwise patterns match the basename or any path segment via
// filepath.Match globs.
type ignoreSet struct{ patterns []string }

func loadIgnore(path string) (*ignoreSet, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ignoreSet{}, nil
	}
	if err != nil {
		return nil, err
	}
	set := &ignoreSet{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		set.patterns = append(set.patterns, line)
	}
	return set, nil
}

// Match reports whether the slash-separated relative path is ignored.
func (s *ignoreSet) Match(rel string) bool {
	base := rel[strings.LastIndex(rel, "/")+1:]
	for _, p := range s.patterns {
		switch {
		case strings.HasSuffix(p, "/"):
			dir := strings.Trim(p, "/")
			if rel == dir || strings.HasPrefix(rel, dir+"/") || strings.Contains(rel, "/"+dir+"/") {
				return true
			}
		case strings.HasPrefix(p, "/"):
			if ok, _ := filepath.Match(strings.TrimPrefix(p, "/"), rel); ok {
				return true
			}
		default:
			if ok, _ := filepath.Match(p, base); ok {
				return true
			}
			if ok, _ := filepath.Match(p, rel); ok {
				return true
			}
		}
	}
	return false
}
