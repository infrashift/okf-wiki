package main

import (
	"strings"
	"testing"

	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/cluster"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

// clusterMeta reads the recorded clustering settings, releasing the store
// before returning. Holding it open would block the next command on the
// database lock rather than testing anything.
func clusterMeta(t *testing.T, root string) store.ClusterMeta {
	t.Helper()
	w := &workspace.Workspace{Root: root}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	meta, err := s.GetClusterMeta()
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

// runCmd executes a graphy subcommand with the process cwd set to root.
// Commands discover their workspace from the cwd, so this is what exercises
// the real flag-parsing and command-wiring path.
func runCmd(t *testing.T, root string, args ...string) error {
	t.Helper()
	t.Chdir(root)
	cmd := rootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	return cmd.Execute()
}

// TestClusterFlagsInheritRecordedSettings is the regression test for the
// reversion bug. `update` bound the clustering flags, but they defaulted to
// graphy's defaults, so a bare update silently re-clustered a louvain graph
// with Leiden — the exact thing binding the flags was meant to prevent.
func TestClusterFlagsInheritRecordedSettings(t *testing.T) {
	root := builtWorkspace(t)

	if err := runCmd(t, root, "build", "--cluster-algorithm", "louvain",
		"--cluster-resolution", "1.5", "--cluster-seed", "7"); err != nil {
		t.Fatalf("build: %v", err)
	}
	meta := clusterMeta(t, root)
	if meta.Algorithm != "louvain" || meta.Resolution != 1.5 || meta.Seed != 7 {
		t.Fatalf("build did not record the requested settings: %+v", meta)
	}

	// A bare update inherits all three.
	if err := runCmd(t, root, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if meta = clusterMeta(t, root); meta.Algorithm != "louvain" || meta.Resolution != 1.5 || meta.Seed != 7 {
		t.Errorf("bare update reverted clustering to %+v, want louvain/1.5/7", meta)
	}

	// An explicit flag still wins — including one that names the default.
	if err := runCmd(t, root, "update", "--cluster-algorithm", "leiden"); err != nil {
		t.Fatalf("update --cluster-algorithm leiden: %v", err)
	}
	meta = clusterMeta(t, root)
	if meta.Algorithm != string(cluster.AlgoLeiden) {
		t.Errorf("explicit --cluster-algorithm leiden was ignored: %+v", meta)
	}
	if meta.Resolution != 1.5 || meta.Seed != 7 {
		t.Errorf("unset flags should still inherit, got %+v", meta)
	}
}

func TestClusterFlagRejectsUnknownAlgorithm(t *testing.T) {
	root := builtWorkspace(t)
	err := runCmd(t, root, "build", "--cluster-algorithm", "spectral")
	if err == nil {
		t.Fatal("an unknown clustering algorithm should be rejected")
	}
	if !strings.Contains(err.Error(), "spectral") {
		t.Errorf("error should name the bad value, got %v", err)
	}
}

// TestLangsScopesWorkNotTheGraph is the regression test for the purge bug:
// --langs drove the delete diff, so building one language deleted every node
// belonging to the others.
func TestLangsScopesWorkNotTheGraph(t *testing.T) {
	root := fixtureRepo(t)
	write(t, root, "svc/app.py", "def main():\n    return compute()\n\ndef compute():\n    return 1\n")
	write(t, root, "docs/guide.md", "# Guide\n\nSee [main](../main.go).\n")
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := runCmd(t, root, "build"); err != nil {
		t.Fatalf("build: %v", err)
	}

	before := languageCounts(t, w)
	for _, lang := range []string{"go", "python", "markdown"} {
		if before[lang] == 0 {
			t.Fatalf("fixture should produce %s nodes, got %v", lang, before)
		}
	}

	if err := runCmd(t, root, "build", "--langs", "go"); err != nil {
		t.Fatalf("build --langs go: %v", err)
	}
	after := languageCounts(t, w)
	for _, lang := range []string{"python", "markdown"} {
		if after[lang] != before[lang] {
			t.Errorf("--langs go changed %s node count from %d to %d; out-of-scope languages must survive",
				lang, before[lang], after[lang])
		}
	}
	if after["go"] == 0 {
		t.Error("--langs go dropped the Go nodes it was supposed to extract")
	}
}

func TestLangsRejectsUnknownLanguage(t *testing.T) {
	root := builtWorkspace(t)
	err := runCmd(t, root, "build", "--langs", "gopher")
	if err == nil {
		t.Fatal("an unknown language should be rejected, not silently wipe the graph")
	}
	if !strings.Contains(err.Error(), "gopher") {
		t.Errorf("error should name the bad language, got %v", err)
	}
	// The error must arrive before anything is purged.
	if counts := languageCounts(t, &workspace.Workspace{Root: root}); counts["go"] == 0 {
		t.Errorf("a rejected --langs must not touch the graph, got %v", counts)
	}
}

func languageCounts(t *testing.T, w *workspace.Workspace) map[string]int {
	t.Helper()
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	counts := map[string]int{}
	if err := s.EachNode(func(n model.Node) error {
		if n.Language != "" {
			counts[n.Language]++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return counts
}

// TestFullRebuildReportsCacheClearsSeparately pins the accounting fix: a --full
// run purges every cached record, and used to report "0 purged" while doing so.
func TestFullRebuildReportsCacheClearsSeparately(t *testing.T) {
	root := builtWorkspace(t)
	w := &workspace.Workspace{Root: root}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	stats, err := build.Run(w, s, build.Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.CacheCleared == 0 {
		t.Error("a --full rebuild must report the records it cleared")
	}
	if stats.FilesPurged != 0 {
		t.Errorf("nothing left the repo, so FilesPurged should be 0, got %d", stats.FilesPurged)
	}
	if stats.FilesExtracted != stats.FilesScanned {
		t.Errorf("a --full rebuild re-extracts everything: extracted %d of %d scanned",
			stats.FilesExtracted, stats.FilesScanned)
	}
}

func TestRootCommandSilencesCobraErrorPrinting(t *testing.T) {
	// Without SilenceErrors, cobra prints the error and main prints it again.
	if !rootCmd().SilenceErrors {
		t.Error("rootCmd must set SilenceErrors; main() prints the error itself")
	}
}
