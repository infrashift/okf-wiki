package bundle

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// DirFS returns an fs.FS for the bundle rooted at dir, like os.DirFS, except
// that it refuses to open a path whose symbolic links lead out of dir. OKF
// v0.3 section 3.2 says a consumer reads only the files under the bundle root
// and does not follow a symbolic link out of it; os.DirFS follows such a link,
// so a bundle downloaded from elsewhere could have Walk read any file on the
// machine. A link that stays inside the bundle still works.
func DirFS(dir string) fs.FS {
	root := dir
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		root = r
	}
	return &containedFS{root: root, dir: dir, fsys: os.DirFS(dir)}
}

type containedFS struct {
	root string // dir with its own links resolved
	dir  string
	fsys fs.FS

	// dirInside caches, per slash-separated directory, whether it resolves
	// inside the bundle. Resolving every path component on every Open cost a
	// search a fifth of its time; a walk opens many files per directory.
	dirInside sync.Map // string -> bool
}

func (c *containedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if !c.inside(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrInvalidPath}
	}
	return c.fsys.Open(name)
}

// inside reports whether name stays in the bundle once its links are
// followed. A name that is not itself a link stays in exactly when its
// directory does, which is cached; only a link is resolved in full.
func (c *containedFS) inside(name string) bool {
	full := filepath.Join(c.dir, filepath.FromSlash(name))
	fi, err := os.Lstat(full)
	if err != nil {
		return true // missing: os.DirFS reports the error itself
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return c.resolvesInside(full)
	}
	if name == "." {
		return true
	}
	d := path.Dir(name)
	if v, ok := c.dirInside.Load(d); ok {
		return v.(bool)
	}
	in := d == "." || c.resolvesInside(filepath.Join(c.dir, filepath.FromSlash(d)))
	c.dirInside.Store(d, in)
	return in
}

func (c *containedFS) resolvesInside(full string) bool {
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		return true // dangling: os.DirFS reports the error itself
	}
	rel, err := filepath.Rel(c.root, resolved)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
