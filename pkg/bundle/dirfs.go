package bundle

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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
	return containedFS{root: root, fsys: os.DirFS(dir), dir: dir}
}

type containedFS struct {
	root string // dir with its own links resolved
	dir  string
	fsys fs.FS
}

func (c containedFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(c.dir, filepath.FromSlash(name)))
	if err == nil {
		rel, rerr := filepath.Rel(c.root, resolved)
		if rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, &fs.PathError{Op: "open", Path: name, Err: ErrInvalidPath}
		}
	}
	return c.fsys.Open(name)
}
