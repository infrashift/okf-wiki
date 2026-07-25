package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/cluster"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// fixtureRepo builds a small cross-referencing repo: two Go packages linked by
// an internal import + cross-file call, and a doc linking a code file.
func fixtureRepo(t *testing.T) (*workspace.Workspace, *store.Store) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/fixture\n\ngo 1.23\n")
	write(t, root, "util/util.go", `package util

// Helper does a thing.
func Helper(x string) string { return x }
`)
	write(t, root, "main.go", `package main

import (
	"fmt"

	"example.com/fixture/util"
)

func main() {
	fmt.Println(run())
}

func run() string {
	return util.Helper("hi")
}
`)
	write(t, root, "docs/guide.md", `# Guide

See [the entry point](../main.go).
`)
	write(t, root, "ci.yaml", `jobs:
  test:
    script: main.go
`)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return w, s
}

func edgeSet(t *testing.T, s *store.Store) map[string]model.Confidence {
	t.Helper()
	set := map[string]model.Confidence{}
	if err := s.EachEdge(func(e model.Edge) error {
		set[e.Source+" -"+string(e.Relation)+"-> "+e.Target] = e.Confidence
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return set
}

func TestBuildResolvesAcrossFilesAndConfidences(t *testing.T) {
	w, s := fixtureRepo(t)
	stats, err := Run(w, s, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.FilesExtracted != 4 {
		t.Errorf("FilesExtracted = %d, want 4", stats.FilesExtracted)
	}

	edges := edgeSet(t, s)

	// Same-file call: main -> run is EXTRACTED.
	if c := edges["go:function:main.go#main -calls-> go:function:main.go#run"]; c != model.ConfExtracted {
		t.Errorf("main->run confidence = %q, want EXTRACTED\nedges: %v", c, edges)
	}
	// Cross-file unique call: run -> util.Helper is INFERRED.
	if c := edges["go:function:main.go#run -calls-> go:function:util/util.go#Helper"]; c != model.ConfInferred {
		t.Errorf("run->Helper confidence = %q, want INFERRED\nedges: %v", c, edges)
	}
	// Internal import resolves to a package node; external import to an external node.
	if c := edges["go:file:main.go# -imports-> go:module:util#util"]; c != model.ConfInferred {
		t.Errorf("internal import missing: %v", edges)
	}
	if c := edges["go:file:main.go# -imports-> go:external:#fmt"]; c != model.ConfInferred {
		t.Errorf("external import missing: %v", edges)
	}
	// Doc link to a code file: EXTRACTED reference onto main.go's file node.
	if c := edges["markdown:doc:docs/guide.md# -references-> go:file:main.go#"]; c != model.ConfExtracted {
		t.Errorf("doc->code reference missing: %v", edges)
	}
	// Config path value resolves to the code file it names.
	if c := edges["yaml:config:ci.yaml#jobs -references-> go:file:main.go#"]; c != model.ConfExtracted {
		t.Errorf("config->code reference missing: %v", edges)
	}

	// External node exists and is marked external.
	ext, err := s.GetNode("go:external:#fmt")
	if err != nil || ext == nil || !ext.External {
		t.Errorf("external fmt node = %+v, %v", ext, err)
	}

	// Degrees were recomputed.
	n, _ := s.GetNode("go:function:util/util.go#Helper")
	if n == nil || n.Degree == 0 {
		t.Errorf("Helper degree = %+v", n)
	}
}

func TestBuildIsIncremental(t *testing.T) {
	w, s := fixtureRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}

	// No changes: nothing re-extracts.
	stats, err := Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesExtracted != 0 {
		t.Errorf("unchanged rebuild extracted %d files, want 0", stats.FilesExtracted)
	}

	// Touch one file: only it re-extracts.
	write(t, w.Root, "util/util.go", `package util

// Helper does a thing.
func Helper(x string) string { return x + "!" }

// Extra is new.
func Extra() {}
`)
	stats, err = Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesExtracted != 1 {
		t.Errorf("after touching one file, extracted %d, want 1", stats.FilesExtracted)
	}
	if n, _ := s.GetNode("go:function:util/util.go#Extra"); n == nil {
		t.Errorf("new function not extracted")
	}

	// Delete a file: its nodes are purged.
	if err := os.Remove(filepath.Join(w.Root, "docs", "guide.md")); err != nil {
		t.Fatal(err)
	}
	stats, err = Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesPurged != 1 {
		t.Errorf("FilesPurged = %d, want 1", stats.FilesPurged)
	}
	if n, _ := s.GetNode("markdown:doc:docs/guide.md#"); n != nil {
		t.Errorf("deleted doc node survived")
	}
	edges := edgeSet(t, s)
	for k := range edges {
		if k == "markdown:doc:docs/guide.md# -references-> go:file:main.go#" {
			t.Errorf("stale doc edge survived: %v", k)
		}
	}
}

func TestBuildFullRebuildMatchesIncremental(t *testing.T) {
	w, s := fixtureRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}
	before := edgeSet(t, s)
	if _, err := Run(w, s, Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	after := edgeSet(t, s)
	if len(before) != len(after) {
		t.Errorf("full rebuild edge count %d != incremental %d", len(after), len(before))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("edge %q: %q != %q", k, after[k], v)
		}
	}
}

func TestBuildResolvesAnsibleRolesAndSQLForeignKeys(t *testing.T) {
	root := t.TempDir()
	write(t, root, "site.yml", `---
- name: Deploy
  hosts: web
  roles:
    - web
    - ghost
`)
	write(t, root, "roles/web/tasks/main.yml", `- name: Install
  package:
    name: nginx
`)
	write(t, root, "db/users.sql", "CREATE TABLE users (id BIGINT PRIMARY KEY);\n")
	write(t, root, "db/orders.sql", "CREATE TABLE orders (\n  user_id BIGINT REFERENCES users (id)\n);\n")

	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}

	edges := edgeSet(t, s)
	// Extracted role resolves to its entry file; unknown role goes external.
	if c := edges["yaml:config:site.yml#Deploy -imports-> yaml:file:roles/web/tasks/main.yml#"]; c != model.ConfInferred {
		t.Errorf("play->role edge missing: %v", edges)
	}
	if c := edges["yaml:config:site.yml#Deploy -imports-> yaml:external:#role:ghost"]; c != model.ConfInferred {
		t.Errorf("unknown role should be external: %v", edges)
	}
	// Cross-file FK resolves through the symbol index.
	if c := edges["sql:struct:db/orders.sql#orders -references-> sql:struct:db/users.sql#users"]; c != model.ConfInferred {
		t.Errorf("orders->users FK missing: %v", edges)
	}
}

// communityStamps snapshots every node's persisted community assignment.
func communityStamps(t *testing.T, s *store.Store) map[string]int {
	t.Helper()
	out := map[string]int{}
	if err := s.EachNode(func(n model.Node) error {
		out[n.ID] = n.Community
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBuildClusteringIsReproducible(t *testing.T) {
	w, s := fixtureRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}
	first := communityStamps(t, s)

	if _, err := Run(w, s, Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	second := communityStamps(t, s)

	if len(first) != len(second) {
		t.Fatalf("node count changed across builds: %d then %d", len(first), len(second))
	}
	for id, c := range first {
		if second[id] != c {
			t.Errorf("%s community %d then %d", id, c, second[id])
		}
	}
}

func TestBuildDefaultsToLeidenAndHonoursTheAlgorithmFlag(t *testing.T) {
	w, s := fixtureRepo(t)

	// A bare Options must mean Leiden — `graphy update` and the MCP update
	// tool both construct one.
	stats, err := Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ClusterAlgorithm != string(cluster.AlgoLeiden) {
		t.Errorf("zero-value Options used %q, want leiden", stats.ClusterAlgorithm)
	}
	meta, err := s.GetClusterMeta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Algorithm != string(cluster.AlgoLeiden) || meta.Resolution != 1 || meta.Seed != 1 {
		t.Errorf("recorded cluster meta = %+v, want leiden/1/1", meta)
	}

	// Opting into Louvain must be recorded, so the report and any later audit
	// reflect what actually ran.
	if _, err := Run(w, s, Options{Cluster: ClusterOptions{Algorithm: cluster.AlgoLouvain}}); err != nil {
		t.Fatal(err)
	}
	meta, err = s.GetClusterMeta()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Algorithm != string(cluster.AlgoLouvain) {
		t.Errorf("recorded algorithm = %q, want louvain", meta.Algorithm)
	}
}

func TestBuildRejectsBadClusterOptions(t *testing.T) {
	w, s := fixtureRepo(t)
	for name, opts := range map[string]ClusterOptions{
		"unknown algorithm":   {Algorithm: "louvian"},
		"negative resolution": {Resolution: -1},
	} {
		if _, err := Run(w, s, Options{Cluster: opts}); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

// rationaleRepo is a repo with a marked comment and an ADR naming the function
// that comment sits on.
func rationaleRepo(t *testing.T) (*workspace.Workspace, *store.Store) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/fixture\n\ngo 1.23\n")
	write(t, root, "store.go", `package fixture

// Save persists a record.
// WHY: writes go through one transaction so a partial batch is never observed.
func Save(key string) error { return nil }
`)
	write(t, root, "docs/adr/0001-use-one-txn.md", `# Use a single transaction

## Decision

All writes go through `+"`Save`"+`, which owns the transaction boundary.
`)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return w, s
}

func TestBuildLinksRationaleToCode(t *testing.T) {
	w, s := rationaleRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}

	var comment, adr string
	if err := s.EachNode(func(n model.Node) error {
		if n.Kind != model.KindRationale {
			return nil
		}
		_, marker, _, ok := model.ParseRationaleQual(qualOfTest(n.ID))
		if !ok {
			t.Errorf("rationale id %q has no parseable qual", n.ID)
			return nil
		}
		switch marker {
		case "WHY":
			comment = n.ID
		case "ADR":
			adr = n.ID
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if comment == "" || adr == "" {
		t.Fatalf("missing rationale nodes: comment=%q adr=%q", comment, adr)
	}

	edges := edgeSet(t, s)
	var fromComment, fromADR model.Confidence
	var sawComment, sawADR bool
	for key, conf := range edges {
		if !strings.Contains(key, "-explains->") {
			continue
		}
		switch {
		case strings.HasPrefix(key, comment+" "):
			fromComment, sawComment = conf, true
		case strings.HasPrefix(key, adr+" "):
			fromADR, sawADR = conf, true
		}
	}
	// A comment resolves in-file and unambiguously; an ADR's named symbol
	// resolves through the global symbol index.
	if !sawComment || fromComment != model.ConfExtracted {
		t.Errorf("comment explains edge = %v (seen=%v), want EXTRACTED", fromComment, sawComment)
	}
	if !sawADR || fromADR != model.ConfInferred {
		t.Errorf("ADR explains edge = %v (seen=%v), want INFERRED", fromADR, sawADR)
	}
}

func TestBuildRationaleIsStableAcrossIncrementalRuns(t *testing.T) {
	w, s := rationaleRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}
	first := edgeSet(t, s)
	firstNodes := communityStamps(t, s)

	if _, err := Run(w, s, Options{}); err != nil { // incremental, nothing changed
		t.Fatal(err)
	}
	if got := len(communityStamps(t, s)); got != len(firstNodes) {
		t.Errorf("node count changed on a no-op rebuild: %d then %d", len(firstNodes), got)
	}
	second := edgeSet(t, s)
	if len(first) != len(second) {
		t.Fatalf("edge count changed on a no-op rebuild: %d then %d", len(first), len(second))
	}
	for k, v := range first {
		if second[k] != v {
			t.Errorf("edge %s: %v then %v", k, v, second[k])
		}
	}
}

// TestBuildFingerprintForcesReExtract covers the hazard that makes rationale
// silently absent: extraction is cached by file content, so new extraction
// capability alone would never invalidate it.
func TestBuildFingerprintForcesReExtract(t *testing.T) {
	w, s := rationaleRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}
	// A second run changes nothing: everything is cached.
	stats, err := Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesExtracted != 0 {
		t.Fatalf("no-op rebuild extracted %d files, want 0", stats.FilesExtracted)
	}

	// Simulate an upgrade: the recorded fingerprint no longer matches.
	if err := s.PutMeta(metaExtractFingerprintKey, "stale"); err != nil {
		t.Fatal(err)
	}
	stats, err = Run(w, s, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.FilesExtracted == 0 {
		t.Error("a changed extraction fingerprint must force a full re-extract")
	}
}

func TestBuildRationaleExcludedFromSymbolIndex(t *testing.T) {
	w, s := rationaleRepo(t)
	if _, err := Run(w, s, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := s.EachNode(func(n model.Node) error {
		if n.Kind != model.KindRationale {
			return nil
		}
		ids, err := s.LookupSymbol(n.Language, n.Label)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if id == n.ID {
				t.Errorf("rationale %q is in the symbol index; a code reference could resolve onto it", n.ID)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func qualOfTest(nodeID string) string {
	if i := strings.LastIndex(nodeID, "#"); i >= 0 {
		return nodeID[i+1:]
	}
	return ""
}
