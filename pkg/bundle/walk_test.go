package bundle_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/infrashift/okf-wiki/pkg/bundle"
)

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

func note(title string) string {
	return "---\ntitle: \"" + title + "\"\ntype: note\ntags: []\n---\nbody of " + title + "\n"
}

// archive is a bundle as a consumer holds it: the project tree, with the
// bundle at .okfw/.
func archive() fstest.MapFS {
	return fstest.MapFS{
		".okfw/index.md":             file("# listing\n"),
		".okfw/log.md":               file("# log\n"),
		".okfw/a.md":                 file(note("A")),
		".okfw/notes/b.md":           file(note("B")),
		".okfw/notes/readme.txt":     file("not markdown"),
		".okfw/notes/ephemeral/c.md": file(note("C")), // nested: part of the bundle
		".okfw/ephemeral/export.md":  file(note("Export")),
		".okfw/.git/x.md":            file(note("Git")),
		".okfw/notes/.hidden/d.md":   file(note("Hidden")),
		".okfw/plain.md":             file("no frontmatter\n"),
		"README.md":                  file("outside the bundle\n"),
	}
}

func sub(t *testing.T, fsys fs.FS) fs.FS {
	t.Helper()
	s, err := fs.Sub(fsys, ".okfw")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func walked(t *testing.T, fsys fs.FS, dir string, lim bundle.Limits) ([]string, error) {
	t.Helper()
	var got []string
	err := bundle.Walk(fsys, dir, lim, func(p string, _ []byte) error {
		got = append(got, p)
		return nil
	})
	return got, err
}

func TestWalkAppliesOkfwRules(t *testing.T) {
	got, err := walked(t, sub(t, archive()), ".", bundle.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	// Lexical order; no reserved files, no hidden directories, no root
	// ephemeral/, no non-markdown. plain.md is passed on: Walk does not parse.
	want := "a.md notes/b.md notes/ephemeral/c.md plain.md"
	if strings.Join(got, " ") != want {
		t.Errorf("walked %v, want %s", got, want)
	}
}

func TestWalkScopedToADirectory(t *testing.T) {
	got, err := walked(t, sub(t, archive()), "notes", bundle.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if want := "notes/b.md notes/ephemeral/c.md"; strings.Join(got, " ") != want {
		t.Errorf("walked %v, want %s", got, want)
	}
	// The rules are judged from the bundle root, not from dir: the root's
	// ephemeral/ stays skipped when it is the directory asked for.
	if got, err := walked(t, sub(t, archive()), "ephemeral", bundle.DefaultLimits); err != nil || len(got) != 0 {
		t.Errorf("walk of ephemeral/ = %v, %v; want nothing", got, err)
	}
}

func TestLoadParsesConcepts(t *testing.T) {
	cs, err := bundle.Load(sub(t, archive()), bundle.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range cs {
		got = append(got, c.ID+"="+c.Title)
	}
	// plain.md has no frontmatter, so it is not a concept.
	if want := "a=A notes/b=B notes/ephemeral/c=C"; strings.Join(got, " ") != want {
		t.Errorf("loaded %v, want %s", got, want)
	}
	if cs[1].Path != "notes/b.md" || cs[1].Body != "body of B\n" || cs[1].Status != "stable" {
		t.Errorf("concept = %+v", cs[1])
	}
}

func TestLimitsMustBePositive(t *testing.T) {
	for _, lim := range []bundle.Limits{{}, {MaxFileBytes: 1}, {MaxFiles: 1}, {MaxFileBytes: -1, MaxFiles: 1}} {
		if _, err := bundle.Load(sub(t, archive()), lim); err == nil {
			t.Errorf("Load with %+v succeeded; a limit left unset must not mean unbounded", lim)
		}
	}
}

func TestMaxFilesCountsEveryFileReached(t *testing.T) {
	// Seven files are reached: a.md, notes/b.md, notes/ephemeral/c.md,
	// notes/readme.txt, plain.md, and index.md and log.md, which count though
	// they hold no concept. The files of skipped directories are not reached.
	fsys := sub(t, archive())
	lim := bundle.Limits{MaxFileBytes: 1 << 10, MaxFiles: 7}
	if _, err := bundle.Load(fsys, lim); err != nil {
		t.Fatalf("7 files at a limit of 7: %v", err)
	}
	lim.MaxFiles = 6
	_, err := bundle.Load(fsys, lim)
	if !errors.Is(err, bundle.ErrTooManyFiles) {
		t.Fatalf("7 files at a limit of 6: err = %v, want ErrTooManyFiles", err)
	}
	var pe *fs.PathError
	if !errors.As(err, &pe) || pe.Path == "" {
		t.Errorf("the error should name the file: %v", err)
	}
}

func TestMaxFileBytesIsExact(t *testing.T) {
	content := note("Sized")
	fsys := fstest.MapFS{"x.md": file(content)}
	n := int64(len(content))
	if _, err := bundle.Load(fsys, bundle.Limits{MaxFileBytes: n, MaxFiles: 1}); err != nil {
		t.Fatalf("a file of exactly MaxFileBytes: %v", err)
	}
	_, err := bundle.Load(fsys, bundle.Limits{MaxFileBytes: n - 1, MaxFiles: 1})
	if !errors.Is(err, bundle.ErrFileTooLarge) {
		t.Fatalf("one byte over: err = %v, want ErrFileTooLarge", err)
	}
	if !strings.Contains(err.Error(), "x.md") {
		t.Errorf("the error should name the file: %v", err)
	}
}

// TestMaxFileBytesDoesNotTrustTheReportedSize: the read itself is bounded, so
// an FS reporting a small size for a large file is still caught.
func TestMaxFileBytesDoesNotTrustTheReportedSize(t *testing.T) {
	fsys := lyingFS{fstest.MapFS{"x.md": file(note(strings.Repeat("big", 100)))}}
	_, err := bundle.Load(fsys, bundle.Limits{MaxFileBytes: 64, MaxFiles: 10})
	if !errors.Is(err, bundle.ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
}

func TestWalkRefusesAnInvalidDir(t *testing.T) {
	for _, dir := range []string{"../x", "/abs", "a/../b", "", "notes/"} {
		_, err := walked(t, sub(t, archive()), dir, bundle.DefaultLimits)
		if !errors.Is(err, bundle.ErrInvalidPath) {
			t.Errorf("Walk(%q): err = %v, want ErrInvalidPath", dir, err)
		}
	}
}

// TestWalkRefusesAPathLeavingTheBundle: fs.WalkDir joins the names an FS
// returns onto their directory, so an FS naming an entry "../x.md" would lead
// the walk out of the bundle.
func TestWalkRefusesAPathLeavingTheBundle(t *testing.T) {
	fsys := escapingFS{fstest.MapFS{"notes/a.md": file(note("A"))}}
	var read []string
	err := bundle.Walk(fsys, "notes", bundle.DefaultLimits, func(p string, _ []byte) error {
		read = append(read, p)
		return nil
	})
	if !errors.Is(err, bundle.ErrInvalidPath) {
		t.Fatalf("err = %v, want ErrInvalidPath", err)
	}
	for _, p := range read {
		if strings.Contains(p, "..") {
			t.Errorf("read %q, outside the bundle", p)
		}
	}
}

func TestWalkStopsOnCallbackError(t *testing.T) {
	stop := errors.New("stop")
	calls := 0
	err := bundle.Walk(sub(t, archive()), ".", bundle.DefaultLimits, func(string, []byte) error {
		calls++
		return stop
	})
	if !errors.Is(err, stop) || calls != 1 {
		t.Errorf("err = %v after %d calls, want stop after 1", err, calls)
	}
}

// TestWalkSkipsAnUnreadableFile as okfw search does: one bad file must not
// hide the rest of the bundle.
func TestWalkSkipsAnUnreadableFile(t *testing.T) {
	fsys := failingFS{fstest.MapFS{"a.md": file(note("A")), "b.md": file(note("B"))}, "a.md"}
	got, err := walked(t, fsys, ".", bundle.DefaultLimits)
	if err != nil || strings.Join(got, " ") != "b.md" {
		t.Errorf("walked %v, %v; want [b.md]", got, err)
	}
}

// lyingFS reports every file as 1 byte long.
type lyingFS struct{ fstest.MapFS }

func (f lyingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	es, err := f.MapFS.ReadDir(name)
	for i, e := range es {
		es[i] = smallEntry{e}
	}
	return es, err
}

type smallEntry struct{ fs.DirEntry }

func (e smallEntry) Info() (fs.FileInfo, error) {
	fi, err := e.DirEntry.Info()
	return smallInfo{fi}, err
}

type smallInfo struct{ fs.FileInfo }

func (smallInfo) Size() int64 { return 1 }

// escapingFS adds an entry named "../escape.md" to every directory listing.
type escapingFS struct{ fstest.MapFS }

func (f escapingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	es, err := f.MapFS.ReadDir(name)
	return append([]fs.DirEntry{namedEntry{"../escape.md"}}, es...), err
}

type namedEntry struct{ name string }

func (e namedEntry) Name() string               { return e.name }
func (e namedEntry) IsDir() bool                { return false }
func (e namedEntry) Type() fs.FileMode          { return 0 }
func (e namedEntry) Info() (fs.FileInfo, error) { return namedInfo(e), nil }

type namedInfo namedEntry

func (i namedInfo) Name() string       { return i.name }
func (i namedInfo) Size() int64        { return 0 }
func (i namedInfo) Mode() fs.FileMode  { return 0 }
func (i namedInfo) ModTime() time.Time { return time.Time{} }
func (i namedInfo) IsDir() bool        { return false }
func (i namedInfo) Sys() any           { return nil }

// failingFS cannot open one file.
type failingFS struct {
	fstest.MapFS
	bad string
}

func (f failingFS) Open(name string) (fs.File, error) {
	if name == f.bad {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.Open(name)
}
