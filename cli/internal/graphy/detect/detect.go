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
	".go":     "go",
	".py":     "python",
	".js":     "javascript",
	".jsx":    "javascript",
	".mjs":    "javascript",
	".cjs":    "javascript",
	".ts":     "typescript",
	".tsx":    "typescript",
	".md":     "markdown",
	".yaml":   "yaml",
	".yml":    "yaml",
	".json":   "json",
	".cue":    "cue",
	".tf":     "terraform",
	".tfvars": "terraform",
	".sql":    "sql",
}

// defaultIgnorePatterns are applied in both listing modes, ahead of any
// .graphyignore entries. ".graphy/" and ".okfw/" are already covered by the
// dot-directory rule below and are named here anyway, because "graphy must
// never read its own output" is a rule worth stating where someone will look
// for it.
var defaultIgnorePatterns = []string{
	".graphy/",
	".okfw/",
	"node_modules/",
	"vendor/",
	"dist/",
	"build/",
	"target/",
}

// hasDotDirSegment reports whether any *directory* component of a
// slash-separated relative path begins with a dot.
//
// Dot-directories hold tooling and state, not the source a graph should
// describe: .terraform/ vendored provider modules, .venv/, .next/, .tox/ — and
// graphy's own .graphy/ and okfw's .okfw/. Both listing modes apply this,
// which is the point. The walk always pruned them locally, but `git ls-files`
// returns whatever the repo tracks, and an okfw bundle's concept markdown is
// tracked by design (okfw init gitignores only .index/, .graph.db, .todos/ and
// ephemeral/). That asymmetry meant every build following a
// `graphy export okfw` re-ingested the bundle as markdown heading nodes, which
// then collided with the very code entities those concepts describe and joined
// the clustering input that exported `parent:` fields depend on.
//
// The trailing component is left alone: a dotfile such as .eslintrc.json is a
// real config file, and either mode may legitimately surface it.
func hasDotDirSegment(rel string) bool {
	segs := strings.Split(rel, "/")
	for _, seg := range segs[:max(len(segs)-1, 0)] {
		if len(seg) > 1 && seg[0] == '.' {
			return true
		}
	}
	return false
}

// Language returns the graphy language for a path ("" when unsupported).
func Language(path string) string {
	return langByExt[strings.ToLower(filepath.Ext(path))]
}

// List discovers, filters, and hashes the extractable files under root.
// Only files with a supported language are returned, sorted by git/walk order.
func List(root string) ([]File, error) {
	ignore, err := loadIgnore(filepath.Join(root, workspace.IgnoreFileName))
	if err != nil {
		return nil, err
	}
	rels, err := listPaths(root, ignore)
	if err != nil {
		return nil, err
	}

	var files []File
	for _, rel := range rels {
		lang := Language(rel)
		if lang == "" || hasDotDirSegment(rel) || ignore.Match(rel) {
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
// else a pruned filesystem walk. Both modes are filtered by the same ignore
// set, so the choice of mode never changes which files a build sees.
func listPaths(root string, ignore *ignoreSet) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		if paths, err := gitListFiles(root); err == nil {
			return paths, nil
		}
	}
	return walkFiles(root, ignore)
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

// walkFiles walks root, pruning ignored directories so large ignored trees are
// never descended into. The pruning rules mirror the filter List applies to
// both modes, so the walk is an optimization rather than a second policy.
func walkFiles(root string, ignore *ignoreSet) ([]string, error) {
	var paths []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		slash := filepath.ToSlash(rel)
		if fi.IsDir() {
			name := fi.Name()
			if p != root && (strings.HasPrefix(name, ".") || ignore.Match(slash)) {
				return filepath.SkipDir
			}
			return nil
		}
		paths = append(paths, slash)
		return nil
	})
	return paths, err
}

// ignoreSet is a minimal gitignore-style matcher: blank lines and #comments
// skipped; a trailing "/" matches a directory prefix; leading "/" anchors to
// the root; otherwise patterns match the basename or any path segment via
// filepath.Match globs.
type ignoreSet struct{ patterns []string }

// newIgnoreSet returns a set seeded with defaultIgnorePatterns.
func newIgnoreSet() *ignoreSet {
	return &ignoreSet{patterns: append([]string(nil), defaultIgnorePatterns...)}
}

func loadIgnore(path string) (*ignoreSet, error) {
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return newIgnoreSet(), nil
	}
	if err != nil {
		return nil, err
	}
	set := newIgnoreSet()
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
			// Matches the directory itself at any depth, and anything under
			// it. The trailing HasSuffix case is what lets walkFiles prune a
			// nested "web/node_modules" before descending into it.
			dir := strings.Trim(p, "/")
			if rel == dir || strings.HasPrefix(rel, dir+"/") ||
				strings.Contains(rel, "/"+dir+"/") || strings.HasSuffix(rel, "/"+dir) {
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
