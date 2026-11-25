package site

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// stubBun writes an executable script named bun into a fresh directory and
// returns that directory, for PATH.
func stubBun(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bun"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPreflight: every way bun can be unusable is reported as its own kind,
// before anything is written, and says how to fix it.
func TestPreflight(t *testing.T) {
	old := versionTimeout
	versionTimeout = 500 * time.Millisecond
	t.Cleanup(func() { versionTimeout = old })

	notExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "bun"), []byte("#!/bin/sh\necho 1.3.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, env, flag, kind string
	}{
		{name: "missing", path: t.TempDir(), kind: "not-found"},
		{name: "too old", path: stubBun(t, "echo 1.1.9"), kind: "too-old"},
		{name: "garbage", path: stubBun(t, "echo 'bun is great'"), kind: "bad-output"},
		{name: "fails", path: stubBun(t, "echo boom >&2; exit 3"), kind: "no-run"},
		{name: "hangs", path: stubBun(t, "exec /bin/sleep 5"), kind: "timeout"},
		{name: "bad OKFW_BUN", path: stubBun(t, "echo 1.3.9"), env: "/nonexistent/bun", kind: "not-found"},
		{name: "OKFW_BUN not executable", path: stubBun(t, "echo 1.3.9"), env: filepath.Join(notExec, "bun"), kind: "not-executable"},
		{name: "bad --bun wins over a good PATH", path: stubBun(t, "echo 1.3.9"), flag: "/nonexistent/bun", kind: "not-found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PATH", tc.path)
			t.Setenv(BunEnv, tc.env)
			_, err := Preflight(tc.flag)
			var pe *PreflightError
			if !errors.As(err, &pe) || pe.Kind != tc.kind {
				t.Fatalf("err = %v, want kind %q", err, tc.kind)
			}
			if !strings.Contains(err.Error(), "https://bun.sh") {
				t.Errorf("the error does not say how to get bun: %v", err)
			}
		})
	}
	t.Run("ok", func(t *testing.T) {
		t.Setenv("PATH", stubBun(t, "echo 1.3.9"))
		t.Setenv(BunEnv, "")
		bun, err := Preflight("")
		if err != nil || bun.Version != (Version{1, 3, 9}) || bun.Source != "PATH" {
			t.Fatalf("Preflight = %+v, %v", bun, err)
		}
	})
	t.Run("OKFW_BUN wins over PATH", func(t *testing.T) {
		t.Setenv("PATH", stubBun(t, "echo 1.1.0"))
		t.Setenv(BunEnv, filepath.Join(stubBun(t, "echo 1.2.0"), "bun"))
		bun, err := Preflight("")
		if err != nil || bun.Source != BunEnv {
			t.Fatalf("Preflight = %+v, %v", bun, err)
		}
	})
}

// writeBundle lays out a small bundle: links of every kind, a deprecated
// concept with superseded_by, an asset, a README that is no concept.
func writeBundle(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj", ".okfw")
	files := map[string]string{
		"index.md": "---\nokf_version: \"0.3\"\ntitle: \"Test: bundle\"\ndescription: \"For tests.\"\n---\n\n# /\n\n* [metrics/](metrics/)\n",
		"metrics/margin.md": "---\ntype: Metric\ntitle: \"Margin: \\\"gross\\\"\"\ntags: [finance]\n" +
			"verified: { by: \"human:ann\", at: \"2026-07-02T00:00:00Z\" }\n" +
			"executor:\n  resource: /skills/run.py\n---\n\nSee [old](old.md), [dir](../metrics/), [chart](/img/c.png) and [gone](nope.md).\n\n```\n[not a link](old.md)\n```\n",
		"metrics/old.md":   "---\ntype: Metric\ntitle: Old\nstatus: deprecated\nsuperseded_by: /metrics/margin.md\n---\n\n# old\n\nOld.\n",
		"README.md":        "# Not a concept\n",
		"skills/run.py":    "print('x')\n",
		"img/c.png":        "png",
		"Weird Dir/A B.md": "---\ntype: Note\n---\n\n# Another heading\n\nBody\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGenerate(t *testing.T) {
	root := writeBundle(t)
	w, err := WorkdirFor(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stats, err := Generate(w)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Concepts != 3 || stats.Dangling != 1 || stats.Assets != 2 {
		t.Errorf("stats = %+v, want 3 concepts (README is none), 1 dangling link, 2 assets", stats)
	}
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(w.Dir, "src", "content", "docs", filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	margin := read("metrics/margin.md")
	for _, want := range []string{
		"](/metrics/old/)",     // a concept link, to its page
		"](/metrics/)",         // a directory link, to its page
		"](/assets/img/c.png)", // an asset, to its published copy
		"](nope.md)",           // a dangling link, as written
		"[not a link](old.md)", // code is not rewritten
		"trust: human-reviewed",
		"href: /assets/skills/run.py",
		"title: 'Margin: \"gross\"'", // yaml-escaped
	} {
		if !strings.Contains(margin, want) {
			t.Errorf("metrics/margin.md lacks %q:\n%s", want, margin)
		}
	}
	old := read("metrics/old.md")
	if !strings.Contains(old, "supersededBy:") || !strings.Contains(old, "href: /metrics/margin/") {
		t.Errorf("superseded_by not resolved:\n%s", old)
	}
	if !strings.Contains(margin, "backlinks:\n        - title: Old") {
		t.Errorf("backlink from old.md (its superseded_by) missing:\n%s", margin)
	}
	if strings.Contains(old, "# old") || !strings.Contains(old, "Old.") {
		t.Errorf("a leading H1 that repeats the title must go (Starlight renders the title), the body stay:\n%s", old)
	}
	weird := read("weird-dir/a-b.md")
	if !strings.Contains(weird, "slug: weird-dir/a-b") {
		t.Error("a path with spaces and capitals was not slugified")
	}
	if !strings.Contains(weird, "# Another heading") {
		t.Errorf("a leading H1 that differs from the title is content and must stay:\n%s", weird)
	}
	if idx := read("index.md"); !strings.Contains(idx, "title: 'Test: bundle'") || !strings.Contains(idx, "](/metrics/)") {
		t.Errorf("root page:\n%s", idx)
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "public", "assets", "skills", "run.py")); err != nil {
		t.Errorf("asset not published: %v", err)
	}
}

// TestRewriteNeverResolvesItsOwnOutput: a concept slug can equal a bundle
// directory's path (tables.md beside tables/), and the second pass must not
// read the first pass's /tables/ as that directory.
func TestRewriteNeverResolvesItsOwnOutput(t *testing.T) {
	target := func(dest, from string) string {
		switch dest {
		case "tables.md":
			return "/tables/"
		case "/tables/", "tables/":
			return "/tables-2/"
		}
		return ""
	}
	got, _ := rewriteBody("[t](tables.md) and [d](tables/)\n", "", target)
	if got != "[t](/tables/) and [d](/tables-2/)\n" {
		t.Errorf("got %q", got)
	}
}

// TestWorkdirIsOutsideTheBundle: OKF v0.3 section 3.2 counts every non-dot
// path under the bundle root, so the build (and its node_modules) must not
// live there.
func TestWorkdirIsOutsideTheBundle(t *testing.T) {
	root := writeBundle(t)
	w, err := WorkdirFor(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if rel, err := filepath.Rel(root, w.Dir); err == nil && !strings.HasPrefix(rel, "..") {
		t.Errorf("workdir %s is inside the bundle %s", w.Dir, root)
	}
}

// TestOnlySiteRunsBun: bun is an optional dependency of one command. Nothing
// outside this package may execute it, so no other command can come to need it.
func TestOnlySiteRunsBun(t *testing.T) {
	moduleRoot := filepath.Join("..", "..", "..")
	execBun := regexp.MustCompile(`exec\.Command(Context)?\([^)]*"bun"`)
	err := filepath.WalkDir(moduleRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "docs" || strings.HasPrefix(d.Name(), ".")) && p != moduleRoot {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.Contains(filepath.ToSlash(p), "cli/internal/site/") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if execBun.Match(data) {
			t.Errorf("%s executes bun; only cli/internal/site may", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBuildWithRealBun builds the site end to end. It needs bun and, the first
// time, the npm registry; CI installs bun so it runs there.
func TestBuildWithRealBun(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		if os.Getenv("OKFW_REQUIRE_BUN") != "" {
			t.Fatal("OKFW_REQUIRE_BUN is set but bun is not on PATH")
		}
		t.Skip("bun is not installed: skipping the end-to-end site build (CI installs bun and runs it)")
	}
	if testing.Short() {
		t.Skip("-short")
	}
	bun, err := Preflight("")
	if err != nil {
		t.Fatal(err)
	}
	w, err := WorkdirFor(writeBundle(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var log strings.Builder
	if _, err := Build(ctx, w, bun, "", &log); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	for _, want := range []string{"index.html", "metrics/margin/index.html", "pagefind/pagefind.js", "assets/skills/run.py"} {
		if _, err := os.Stat(filepath.Join(w.DistDir(), filepath.FromSlash(want))); err != nil {
			t.Errorf("dist lacks %s", want)
		}
	}
	page, _ := os.ReadFile(filepath.Join(w.DistDir(), "metrics", "old", "index.html"))
	if !strings.Contains(string(page), `Superseded by <a href="/metrics/margin/">`) {
		t.Error("the deprecated page has no superseded-by banner")
	}
}
