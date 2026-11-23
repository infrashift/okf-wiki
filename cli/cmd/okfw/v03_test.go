package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/infrashift/okf-wiki/cli/internal/bundle"
	"github.com/infrashift/okf-wiki/internal/config"
)

// TestInitWritesBundleMetadata: OKF v0.3 section 8.1 metadata goes in the
// root index.md, stamped okf_version "0.3"; the title defaults to the project
// directory's name.
func TestInitWritesBundleMetadata(t *testing.T) {
	project := filepath.Join(t.TempDir(), "billing")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatal(err)
	}
	if err := runInitWith(project, bundle.BundleMeta{Description: "Billing runbooks.", License: "CC-BY-4.0"}); err != nil {
		t.Fatal(err)
	}
	meta, err := bundle.ReadBundleMeta(filepath.Join(project, config.WikiDirName))
	if err != nil {
		t.Fatal(err)
	}
	want := bundle.BundleMeta{OKFVersion: "0.3", Title: "billing", Description: "Billing runbooks.", License: "CC-BY-4.0"}
	if meta != want {
		t.Errorf("meta = %+v, want %+v", meta, want)
	}
	gi, _ := os.ReadFile(filepath.Join(project, config.WikiDirName, ".gitignore"))
	if !strings.Contains(string(gi), ".ephemeral/") {
		t.Errorf(".gitignore = %q, want the dot-named scratch directory", gi)
	}
}

// TestRootFrontmatterSurvivesIndexRegeneration: section 8.1 — a producer that
// regenerates index.md MUST keep the bundle-root frontmatter, unknown keys and
// comments included, and okfw never moves okf_version on an ordinary write.
func TestRootFrontmatterSurvivesIndexRegeneration(t *testing.T) {
	cfg := bundleAt(t)
	if _, err := runCmd(t, rootCmd(), "set-bundle-meta", "--description", "What it is for."); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfg.RootPath, "index.md")
	data, _ := os.ReadFile(p)
	data = []byte(strings.Replace(string(data), "okf_version: \"0.3\"", "okf_version: \"0.2\"\n# a comment\nx-acme-owner: \"fpa\"", 1))
	if err := os.WriteFile(p, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, rootCmd(), "create", "A concept"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	for _, want := range []string{`okf_version: "0.2"`, "# a comment", `x-acme-owner: "fpa"`, `description: "What it is for."`, "A concept"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("root index.md lost %q:\n%s", want, got)
		}
	}
}

// TestSetLifecycleSupersededBy: section 5.4 — superseded_by is written as the
// replacement's bundle-absolute path, needs deprecated, and is dropped when
// the concept leaves deprecated.
func TestSetLifecycleSupersededBy(t *testing.T) {
	cfg := bundleAt(t)
	writeConceptFile(t, cfg.RootPath, "metrics/old.md", conceptContent("019680f0-7e00-7abc-8def-b00000000001", "Old", "Metric", "# Old"))
	writeConceptFile(t, cfg.RootPath, "metrics/new.md", conceptContent("019680f0-7e00-7abc-8def-b00000000002", "New", "Metric", "# New"))

	if _, err := runCmd(t, rootCmd(), "set-lifecycle", "metrics/old.md", "stable", "--superseded-by", "metrics/new.md"); err == nil {
		t.Error("superseded_by without deprecated was accepted")
	}
	if out, err := runCmd(t, rootCmd(), "set-lifecycle", "metrics/old.md", "deprecated", "--superseded-by", "019680f0-7e00-7abc-8def-b00000000002"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	old := filepath.Join(cfg.RootPath, "metrics", "old.md")
	z, err := config.ParseAndValidate(mustRead(t, old))
	if err != nil {
		t.Fatal(err)
	}
	if z.SupersededBy != "/metrics/new.md" || z.LifecycleStatus() != "deprecated" {
		t.Errorf("status %q superseded_by %q", z.LifecycleStatus(), z.SupersededBy)
	}
	// It is a link: moving the replacement retargets it.
	if _, err := runCmdIn(t, rootCmd(), "", "move", "metrics/new.md", "kpis/"); err != nil {
		t.Fatal(err)
	}
	if z, _ := config.ParseFrontmatter(mustRead(t, old)); z.SupersededBy != "/kpis/new.md" {
		t.Errorf("after the move superseded_by = %q", z.SupersededBy)
	}
	if _, err := runCmd(t, rootCmd(), "set-lifecycle", "metrics/old.md", "stable"); err != nil {
		t.Fatal(err)
	}
	if z, _ := config.ParseFrontmatter(mustRead(t, old)); z.SupersededBy != "" {
		t.Errorf("superseded_by survived leaving deprecated: %q", z.SupersededBy)
	}
}

// TestMetadataEditsKeepAReviewCurrent: a review counts only while it is at or
// after generated.at (section 5.3). Tag and lifecycle edits must not move
// generated.at; recording a new generation must.
func TestMetadataEditsKeepAReviewCurrent(t *testing.T) {
	cfg := bundleAt(t)
	p := writeConceptFile(t, cfg.RootPath, "c.md", conceptContent("019680f0-7e00-7abc-8def-c00000000001", "C", "note", "# C"))
	steps := [][]string{
		{"set-generated", "c.md", "--by", "agent/1", "--at", "2026-01-01T00:00:00Z"},
		{"verify", "c.md", "--by", "human:alice", "--at", "2026-01-02T00:00:00Z"},
		{"add-tags", "c.md", "finance"},
		{"set-lifecycle", "c.md", "draft"},
		{"add-source", "c.md", "--resource", "https://example.com"},
	}
	for _, s := range steps {
		if out, err := runCmd(t, rootCmd(), s...); err != nil {
			t.Fatalf("%v: %v\n%s", s, err, out)
		}
	}
	z, _ := config.ParseFrontmatter(mustRead(t, p))
	if z.TrustTier() != config.TrustHumanReviewed {
		t.Fatalf("tier after metadata edits = %q, want human-reviewed", z.TrustTier())
	}
	if _, err := runCmd(t, rootCmd(), "set-generated", "c.md", "--by", "agent/2", "--at", "2026-01-03T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if z, _ := config.ParseFrontmatter(mustRead(t, p)); z.TrustTier() != config.TrustUnverified {
		t.Errorf("tier after a new generation = %q, want unverified", z.TrustTier())
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// legacyKeyed is a concept as okfw wrote it before its keys moved to x-okfw-*.
const legacyKeyed = "---\nid: \"019680f0-7e00-7abc-8def-d00000000001\"\ntitle: \"Old\"\ntype: \"note\"\n" +
	"project: \"p\"\ntags: []\ncreated: \"2026-01-01T00:00:00Z\"\ntimestamp: \"2026-01-02T00:00:00Z\"\n---\n\n# Old\n"

// TestMigrateMovesOkfwKeysToTheNamespace: migrate renames the bare keys in
// place, leaves the timestamp's value alone, and the result validates.
func TestMigrateMovesOkfwKeysToTheNamespace(t *testing.T) {
	cfg := bundleAt(t)
	p := writeConceptFile(t, cfg.RootPath, "old.md", legacyKeyed)
	foreign := writeConceptFile(t, cfg.RootPath, "foreign.md", "---\ntype: Table\nid: orders\n---\n\nx\n")
	if _, err := captureStdout(t, func() error { return runMigrate(cfg, cfg.RootPath, false) }); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, p))
	want := strings.NewReplacer("\nid:", "\nx-okfw-id:", "\nproject:", "\nx-okfw-project:",
		"\ncreated:", "\nx-okfw-created:", "\ntimestamp:", "\nx-okfw-timestamp:").Replace(legacyKeyed)
	if got != want {
		t.Errorf("migrated:\n%s\nwant:\n%s", got, want)
	}
	if _, err := config.ParseAndValidate([]byte(got)); err != nil {
		t.Errorf("the migrated concept does not validate: %v", err)
	}
	if f := string(mustRead(t, foreign)); !strings.Contains(f, "\nid: orders\n") {
		t.Errorf("another producer's id was relabelled:\n%s", f)
	}
}

// TestAnEditUpgradesTheKeysOfItsFile: a writer touching an unmigrated
// concept renames its keys rather than adding x-okfw-timestamp beside a
// stale timestamp.
func TestAnEditUpgradesTheKeysOfItsFile(t *testing.T) {
	cfg := bundleAt(t)
	p := writeConceptFile(t, cfg.RootPath, "old.md", legacyKeyed)
	if _, err := runCmd(t, rootCmd(), "add-tags", "old.md", "kept"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, p))
	if strings.Contains(got, "\ntimestamp:") || strings.Count(got, "x-okfw-timestamp:") != 1 || !strings.Contains(got, "x-okfw-id:") {
		t.Errorf("keys not upgraded in place:\n%s", got)
	}
	if _, err := config.ParseAndValidate([]byte(got)); err != nil {
		t.Errorf("edited concept does not validate: %v", err)
	}
}
