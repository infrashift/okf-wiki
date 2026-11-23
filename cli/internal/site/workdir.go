package site

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The Astro + Starlight project the site is built with, pinned by its
// bun.lock. okfw fills in okfw-site.json and src/content/docs/ per build.
//
//go:embed template
var templateFS embed.FS

const templateRoot = "template"

// templateStamp names the file in a workdir recording which embedded template
// it holds, so a new okfw re-materializes it and an unchanged one does not.
const templateStamp = ".okfw-template"

// Workdir is where a bundle's site is built: outside the bundle, in the
// user's cache directory. OKF v0.3 section 3.2 counts every non-dot path under
// the bundle root as part of it, so a node_modules there would put thousands
// of foreign .md files in front of every OKF consumer.
type Workdir struct {
	Dir        string // the Astro project
	BundleRoot string
}

// WorkdirFor returns the build directory for the bundle at root, keyed by a
// hash of its absolute path. cacheDir overrides os.UserCacheDir (tests).
func WorkdirFor(root, cacheDir string) (*Workdir, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if cacheDir == "" {
		if cacheDir, err = os.UserCacheDir(); err != nil {
			return nil, fmt.Errorf("no cache directory for the site build: %w", err)
		}
	}
	sum := sha256.Sum256([]byte(abs))
	return &Workdir{
		Dir:        filepath.Join(cacheDir, "okfw", "site", hex.EncodeToString(sum[:])[:12]),
		BundleRoot: abs,
	}, nil
}

// TemplateHash identifies the embedded template's content.
func TemplateHash() string {
	h := sha256.New()
	var names []string
	_ = fs.WalkDir(templateFS, templateRoot, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	for _, n := range names {
		data, _ := templateFS.ReadFile(n)
		fmt.Fprintf(h, "%s\x00%d\x00", n, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Prepare writes the template into the workdir and installs its dependencies,
// unless the workdir already holds this template with its node_modules. It
// returns whether it ran bun install. bun install needs registry access the
// first time; afterwards bun's own cache makes it fast and offline.
func (w *Workdir) Prepare(ctx context.Context, bun *Bun, log io.Writer) (bool, error) {
	want := TemplateHash()
	have, _ := os.ReadFile(filepath.Join(w.Dir, templateStamp))
	_, nmErr := os.Stat(filepath.Join(w.Dir, "node_modules", "astro", "package.json"))
	if strings.TrimSpace(string(have)) == want && nmErr == nil {
		return false, nil
	}
	if err := os.MkdirAll(w.Dir, 0o755); err != nil {
		return false, err
	}
	err := fs.WalkDir(templateFS, templateRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, templateRoot), "/")
		out := filepath.Join(w.Dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		return false, fmt.Errorf("writing the site template: %w", err)
	}
	if err := w.run(ctx, bun, log, "install", "--frozen-lockfile"); err != nil {
		return true, fmt.Errorf("bun install failed: %w (the first build downloads Astro and Starlight, so it needs access to the npm registry)", err)
	}
	return true, os.WriteFile(filepath.Join(w.Dir, templateStamp), []byte(want+"\n"), 0o644)
}

// run runs bun in the workdir, streaming its output to log.
func (w *Workdir) run(ctx context.Context, bun *Bun, log io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, bun.Path, args...)
	cmd.Dir = w.Dir
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Env = append(os.Environ(), "ASTRO_TELEMETRY_DISABLED=1", "NO_COLOR=1")
	return cmd.Run()
}

// output runs bun in the workdir and returns its combined output.
func (w *Workdir) output(ctx context.Context, bun *Bun, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, bun.Path, args...)
	cmd.Dir = w.Dir
	cmd.Env = append(os.Environ(), "ASTRO_TELEMETRY_DISABLED=1", "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// DistDir is the built site.
func (w *Workdir) DistDir() string { return filepath.Join(w.Dir, "dist") }

// copyTree copies the directory src to dst, replacing dst.
func copyTree(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
}
