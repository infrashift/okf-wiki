package bundle

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strings"

	"github.com/infrashift/okf-wiki/internal/config"
)

// Limits bounds a Walk or a Load. Both fields must be positive: the zero
// value is rejected, so a caller cannot read a bundle unbounded by accident.
type Limits struct {
	// MaxFileBytes is the size of the largest concept file read. A larger one
	// fails the walk with ErrFileTooLarge.
	MaxFileBytes int64
	// MaxFiles is the most files a walk visits. Every non-directory entry the
	// walk reaches counts, whether or not it holds a concept; the contents of
	// skipped directories are not reached. One more fails the walk with
	// ErrTooManyFiles.
	MaxFiles int
}

// DefaultLimits are 1 MiB per file and 4096 files.
var DefaultLimits = Limits{MaxFileBytes: 1 << 20, MaxFiles: 4096}

// Errors a walk fails with. Each is returned inside an *fs.PathError that
// names the file, so test for them with errors.Is.
var (
	// ErrFileTooLarge: a concept file is larger than Limits.MaxFileBytes.
	ErrFileTooLarge = errors.New("file exceeds the size limit")
	// ErrTooManyFiles: the bundle has more files than Limits.MaxFiles.
	ErrTooManyFiles = errors.New("bundle exceeds the file-count limit")
	// ErrInvalidPath: the walk's dir, or a path the FS returned, is not a
	// valid fs.FS path inside the directory walked: it is absolute, holds a
	// "." or ".." element, or leaves the directory.
	ErrInvalidPath = errors.New("path is not inside the bundle")

	// ErrNotConcept is Parse's error for a .md file with no non-empty `type`
	// (OKF v0.3 section 3.2): a README.md or SKILL.md a bundle may hold.
	ErrNotConcept = errors.New("not a concept: no type in the frontmatter")
)

// Walk calls fn for each concept file at or below dir in the bundle at the
// root of fsys, in lexical order. Use "." for dir to walk the whole bundle,
// and fs.Sub to root fsys at the bundle (the .okfw directory). fn receives the
// file's slash-separated path relative to the bundle root and its content; an
// error from fn stops the walk and is returned.
//
// The walk applies okfw's rules. It skips hidden directories and the root's
// ephemeral/ directory, and it passes fn only .md files other than the
// reserved index.md and log.md. It does not parse them, so fn also receives
// files that hold no concept.
//
// Every read is bounded by lim. A file that cannot be opened or read is
// skipped, as okfw search skips it. Walk only names paths inside dir and
// never follows a symbolic link to a directory. It opens every file through
// fsys, so whether a symbolic link to a file is followed is up to fsys: an
// in-memory FS that holds no links cannot be escaped, whereas os.DirFS follows
// a link out of its directory. Use DirFS for a bundle on disk: it refuses one,
// as OKF v0.3 section 3.2 requires.
func Walk(fsys fs.FS, dir string, lim Limits, fn func(path string, content []byte) error) error {
	if lim.MaxFileBytes <= 0 || lim.MaxFiles <= 0 {
		return fmt.Errorf("bundle: limits must be positive, got %+v", lim)
	}
	if !fs.ValidPath(dir) {
		return &fs.PathError{Op: "walk", Path: dir, Err: ErrInvalidPath}
	}
	files := 0
	return fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !inside(dir, p) {
			return &fs.PathError{Op: "walk", Path: p, Err: ErrInvalidPath}
		}
		if d.IsDir() {
			if config.SkipBundleDir(p) {
				return fs.SkipDir
			}
			return nil
		}
		if files++; files > lim.MaxFiles {
			return &fs.PathError{Op: "walk", Path: p, Err: ErrTooManyFiles}
		}
		if !config.IsConceptFile(d.Name()) {
			return nil
		}
		content, err := readBounded(fsys, p, d, lim.MaxFileBytes)
		if errors.Is(err, ErrFileTooLarge) {
			return err
		}
		if err != nil {
			return nil // unreadable: skip it, as okfw search does
		}
		return fn(p, content)
	})
}

// inside reports whether p, a path fs.WalkDir produced, is a valid path at or
// below dir. fs.WalkDir joins each name an FS returns onto its directory, so
// an FS that returned "../x" as a name would otherwise lead the walk out.
func inside(dir, p string) bool {
	if !fs.ValidPath(p) {
		return false
	}
	return dir == "." || p == dir || strings.HasPrefix(p, dir+"/")
}

// readBounded reads the file at p, failing with ErrFileTooLarge past limit
// bytes. The size the directory entry reports is checked first, which is
// cheap, and the read itself is bounded, so an FS whose reported size is wrong
// cannot get past the limit.
func readBounded(fsys fs.FS, p string, d fs.DirEntry, limit int64) ([]byte, error) {
	tooLarge := &fs.PathError{Op: "read", Path: p, Err: ErrFileTooLarge}
	if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() && fi.Size() > limit {
		return nil, tooLarge
	}
	f, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	n := limit
	if n < math.MaxInt64 {
		n++ // one byte more than allowed tells a file at the limit from one past it
	}
	content, err := io.ReadAll(io.LimitReader(f, n))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, tooLarge
	}
	return content, nil
}

// Load parses every concept in the bundle at the root of fsys, in lexical
// order of Path. A file whose frontmatter cannot be parsed is not a concept
// and is left out, as okfw search leaves it out; Load does not apply the
// strict Validate check. It fails as Walk fails, and returns no concepts then.
func Load(fsys fs.FS, lim Limits) ([]*Concept, error) {
	var out []*Concept
	err := Walk(fsys, ".", lim, func(p string, content []byte) error {
		if c, err := Parse(p, content); err == nil {
			out = append(out, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
