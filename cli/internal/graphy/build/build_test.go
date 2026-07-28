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
	// Doc link to a code file: EXTRACTED reference onto main.go's file node,
	// owned by the enclosing H1 rather than the document, so a section links to
	// what it describes.
	if c := edges["markdown:heading:docs/guide.md#guide -references-> go:file:main.go#"]; c != model.ConfExtracted {
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

// TestBuildDropsAmbiguousTailMatchesInsteadOfFanningOut: three types each with
// an Error method, and a caller doing err.Error(). The bare-tail lookup finds
// all three, but the qualifier that said which one was meant has already been
// discarded — so these are not rival readings of the call, they are two
// unrelated methods that happen to share a name. Fanning out AMBIGUOUS over
// them made the three error types the largest, most-connected entities in this
// repo's own graph and the most expensive concepts in its exported bundle.
func TestBuildDropsAmbiguousTailMatchesInsteadOfFanningOut(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/amb\n\ngo 1.23\n")
	write(t, root, "errs/errs.go", `package errs

type A struct{}

func (a *A) Error() string { return "a" }

type B struct{}

func (b *B) Error() string { return "b" }

type C struct{}

func (c *C) Error() string { return "c" }
`)
	write(t, root, "caller/caller.go", `package caller

func Report(err error) string {
	return err.Error()
}
`)
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
		t.Fatalf("Run: %v", err)
	}

	for id, conf := range edgeSet(t, s) {
		if strings.Contains(id, "-calls->") && strings.Contains(id, ".Error") {
			t.Errorf("err.Error() fanned out onto a declared method: %s (%s)", id, conf)
		}
	}
	// And the error methods keep only their structural edges (contains from the
	// file, defines from their type) rather than becoming call hubs.
	for _, q := range []string{"A.Error", "B.Error", "C.Error"} {
		id := "go:method:errs/errs.go#" + q
		in, err := s.InEdges(id)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range in {
			if e.Relation == model.RelCalls {
				t.Errorf("%s has an incoming call edge from %s (%s)", q, e.Source, e.Confidence)
			}
		}
	}
}

// TestBuildScopesQualifiedGoCallsToTheImportedPackage: the flip side of the
// rule above. A call qualified by a package the file actually imports keeps its
// qualifier, so it resolves exactly instead of being dropped as an ambiguous
// tail match — even though two packages declare New.
func TestBuildScopesQualifiedGoCallsToTheImportedPackage(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/scope\n\ngo 1.23\n")
	write(t, root, "store/store.go", "package store\n\nfunc New() int { return 1 }\n")
	write(t, root, "cache/cache.go", "package cache\n\nfunc New() int { return 2 }\n")
	write(t, root, "app/app.go", `package app

import "example.com/scope/store"

func Boot() int {
	return store.New()
}
`)
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
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	const want = "go:function:app/app.go#Boot -calls-> go:function:store/store.go#New"
	if _, ok := edges[want]; !ok {
		t.Errorf("qualified call did not resolve to the imported package\nedges: %v", edges)
	}
	const wrong = "go:function:app/app.go#Boot -calls-> go:function:cache/cache.go#New"
	if _, ok := edges[wrong]; ok {
		t.Errorf("qualified call leaked into a package the file does not import")
	}
}

// TestBuildInfersGoImplements: Go interface satisfaction is structural, so no
// declaration anywhere names the implementor and the only evidence is the two
// method sets. Before this, Go contributed zero implements edges.
func TestBuildInfersGoImplements(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/impl\n\ngo 1.23\n")
	write(t, root, "api/api.go", `package api

// Store reads and writes.
type Store interface {
	Get(k string) string
	Put(k, v string) error
}

// Reader only reads.
type Reader interface {
	Get(k string) string
}

// Empty is satisfied by everything and must never produce an edge.
type Empty interface{}
`)
	write(t, root, "mem/mem.go", `package mem

type Memory struct{}

func (m *Memory) Get(k string) string { return "" }

func (m *Memory) Put(k, v string) error { return nil }

type ReadOnly struct{}

func (r *ReadOnly) Get(k string) string { return "" }
`)
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
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	want := []string{
		"go:struct:mem/mem.go#Memory -implements-> go:interface:api/api.go#Store",
		"go:struct:mem/mem.go#Memory -implements-> go:interface:api/api.go#Reader",
		"go:struct:mem/mem.go#ReadOnly -implements-> go:interface:api/api.go#Reader",
	}
	for _, e := range want {
		if edges[e] != model.ConfInferred {
			t.Errorf("missing %s\nedges: %v", e, edges)
		}
	}
	// A partial method set is not satisfaction.
	if _, ok := edges["go:struct:mem/mem.go#ReadOnly -implements-> go:interface:api/api.go#Store"]; ok {
		t.Error("ReadOnly lacks Put but was reported as implementing Store")
	}
	// An empty interface would otherwise be implemented by every type alive.
	for id := range edges {
		if strings.Contains(id, "#Empty") && strings.Contains(id, "-implements->") {
			t.Errorf("empty interface produced %s", id)
		}
	}
}

// TestBuildLinksTestsToSubjects covers the tests relation and the Test marking.
func TestBuildLinksTestsToSubjects(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/tl\n\ngo 1.23\n")
	write(t, root, "calc/calc.go", `package calc

func Add(a, b int) int { return a + b }

func runTotals() int { return 0 }

type Machine struct{}

func (m *Machine) Run() int { return 0 }
`)
	write(t, root, "calc/calc_test.go", `package calc

import "testing"

func TestAdd(t *testing.T) {}

func TestMachine_Run(t *testing.T) {}

func TestAdd_returnsZeroOnEmpty(t *testing.T) {}

func TestRunTotalsIgnoresNegatives(t *testing.T) {}

func helperNotATest() {}
`)
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
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	for _, e := range []string{
		"go:function:calc/calc_test.go#TestAdd -tests-> go:function:calc/calc.go#Add",
		// TestType_Method: the underscore stands in for the symbol index's dot.
		"go:function:calc/calc_test.go#TestMachine_Run -tests-> go:method:calc/calc.go#Machine.Run",
		// A lowercase suffix is a subtest label, not a method name.
		"go:function:calc/calc_test.go#TestAdd_returnsZeroOnEmpty -tests-> go:function:calc/calc.go#Add",
		// The dominant convention states the scenario in the name, so the
		// subject is the longest CamelCase prefix that names a real symbol —
		// lower-cased first letter included, since the convention capitalizes
		// unexported subjects too.
		"go:function:calc/calc_test.go#TestRunTotalsIgnoresNegatives -tests-> go:function:calc/calc.go#runTotals",
	} {
		if _, ok := edges[e]; !ok {
			t.Errorf("missing %s\nedges: %v", e, edges)
		}
	}

	// Test entities are marked, production ones are not — degree and the
	// community partition both read this.
	for id, wantTest := range map[string]bool{
		"go:function:calc/calc_test.go#TestAdd":        true,
		"go:function:calc/calc_test.go#helperNotATest": true,
		"go:function:calc/calc.go#Add":                 false,
	} {
		n, err := s.GetNode(id)
		if err != nil || n == nil {
			t.Fatalf("missing node %s: %v", id, err)
		}
		if n.Test != wantTest {
			t.Errorf("%s Test = %v, want %v", id, n.Test, wantTest)
		}
	}
}

// TestBuildReadsManifestDependencies: every call into third-party code is
// dropped by resolution, so a manifest is the only place a graph can learn what
// the repo relies on but does not own.
func TestBuildReadsManifestDependencies(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", `module example.com/dep

go 1.23

require (
	github.com/spf13/cobra v1.8.0
	github.com/tree-sitter/tree-sitter-go v0.25.0
)

require golang.org/x/sys v0.20.0 // indirect
`)
	write(t, root, "main.go", `package main

import (
	"github.com/spf13/cobra"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func main() { _ = cobra.Command{}; _ = tsgo.Language }
`)
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
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	const owner = "go:module:go.mod#example.com/dep"
	for _, dep := range []string{"github.com/spf13/cobra", "github.com/tree-sitter/tree-sitter-go"} {
		e := owner + " -depends_on-> go:external:#" + dep
		if edges[e] != model.ConfExtracted {
			t.Errorf("missing %s\nedges: %v", e, edges)
		}
	}
	// Indirect requirements are the build's dependencies, not this repo's.
	for id := range edges {
		if strings.Contains(id, "golang.org/x/sys") {
			t.Errorf("indirect requirement leaked: %s", id)
		}
	}
	// A module and the subpackage actually imported are the same dependency
	// spelled two ways; joining them is what stops the module looking unused.
	const sub = "go:external:#github.com/tree-sitter/tree-sitter-go -contains-> go:external:#github.com/tree-sitter/tree-sitter-go/bindings/go"
	if _, ok := edges[sub]; !ok {
		t.Errorf("module does not own its imported subpackage\nedges: %v", edges)
	}
	// The declared version reaches the node even though import resolution
	// created it first.
	n, err := s.GetNode("go:external:#github.com/spf13/cobra")
	if err != nil || n == nil {
		t.Fatalf("cobra node missing: %v", err)
	}
	if n.Doc != "v1.8.0" {
		t.Errorf("cobra version = %q, want v1.8.0", n.Doc)
	}
}

// TestBuildResolvesCallsThroughAStructField pins the defect that made
// `graphy callers` useless on real Go code: a call through a struct field
// (`s.engine.Evaluate`) reaches the symbol index as the bare tail `Evaluate`,
// and the tail rule drops it whenever more than one type declares that method.
//
// This is the dominant shape for one subsystem reaching another, and it fails
// hardest on the names worth querying — central verbs like Evaluate and
// Validate are exactly the ones that recur across layers. Measured on a real
// 24k-LOC repo, `Engine.Evaluate` and `Engine.Validate` resolved zero callers
// while single-owner names resolved 32 and 45.
func TestBuildResolvesCallsThroughAStructField(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/recv\n\ngo 1.23\n")
	write(t, root, "engine/engine.go", `package engine

type Engine struct{}

// Evaluate is the entry point under test.
func (e *Engine) Evaluate(in string) string { return in }
`)
	// A second Evaluate is what makes the bare tail ambiguous, and is why the
	// old rule dropped the call rather than tagging it AMBIGUOUS.
	write(t, root, "client/client.go", `package client

type Client struct{}

func (c *Client) Evaluate(in string) string { return in }
`)
	write(t, root, "server/server.go", `package server

import "example.com/recv/engine"

type Server struct {
	engine *engine.Engine
}

func (s *Server) handle(in string) string {
	return s.engine.Evaluate(in)
}

func (s *Server) delegate(in string) string {
	return s.handle(in)
}
`)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stats, err := Run(w, s, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	const want = "go:method:server/server.go#Server.handle -calls-> go:method:engine/engine.go#Engine.Evaluate"
	if _, ok := edges[want]; !ok {
		t.Errorf("call through a struct field did not resolve\nwant: %s\nedges: %v", want, edges)
	}
	// The whole point of scoping is that it does not fan out: the identically
	// named method on an unrelated type must not also be claimed.
	const wrong = "go:method:server/server.go#Server.handle -calls-> go:method:client/client.go#Client.Evaluate"
	if _, ok := edges[wrong]; ok {
		t.Errorf("receiver scoping leaked onto an unrelated type's same-named method")
	}
	// A bare `s.method()` call names the receiver's own type.
	const own = "go:method:server/server.go#Server.delegate -calls-> go:method:server/server.go#Server.handle"
	if _, ok := edges[own]; !ok {
		t.Errorf("call on the method's own receiver did not resolve\nwant: %s", own)
	}
	if stats.RefsReceiverScoped == 0 {
		t.Error("RefsReceiverScoped not counted")
	}
}

// TestReceiverScopingRequiresTheReceiverVariable keeps the rule sound: the head
// segment must be the enclosing method's receiver. Without that check any
// `x.engine.Evaluate` would be read as the enclosing type's `engine` field no
// matter what `x` actually is.
func TestReceiverScopingRequiresTheReceiverVariable(t *testing.T) {
	nodes := map[string]model.Node{
		"m": {ID: "m", Label: "Server.handle", Kind: model.KindMethod, Language: "go", Recv: "s"},
		"f": {ID: "f", Label: "Boot", Kind: model.KindFunction, Language: "go"},
	}
	fields := map[string]map[string]string{"Server": {"engine": "Engine"}}

	first := func(ids []string) string {
		if len(ids) == 0 {
			return ""
		}
		return ids[0]
	}
	if got := first(goReceiverQualified(model.Ref{FromID: "m", Name: "s.engine.Evaluate"}, nodes, fields, nil)); got != "Engine.Evaluate" {
		t.Errorf("receiver call: got %q, want Engine.Evaluate", got)
	}
	if got := goReceiverQualified(model.Ref{FromID: "m", Name: "other.engine.Evaluate"}, nodes, fields, nil); got != nil {
		t.Errorf("non-receiver head must not scope: got %q", got)
	}
	if got := goReceiverQualified(model.Ref{FromID: "f", Name: "s.engine.Evaluate"}, nodes, fields, nil); got != nil {
		t.Errorf("plain function has no receiver: got %q", got)
	}
	if got := goReceiverQualified(model.Ref{FromID: "m", Name: "s.a.b.Evaluate"}, nodes, fields, nil); got != nil {
		t.Errorf("deep chain must not be followed: got %q", got)
	}
	if got := goReceiverQualified(model.Ref{FromID: "m", Name: "s.unknown.Evaluate"}, nodes, fields, nil); got != nil {
		t.Errorf("unknown field must not scope: got %q", got)
	}
}

// TestReceiverScopingFollowsMethodPromotion: `b.Get()` on a type that embeds
// S3Backend names S3Backend.Get, and nothing on the outer type declares it.
// Without promotion this shape resolved before the fix (via the same-file
// shortcut) and would have regressed to nothing after it.
func TestReceiverScopingFollowsMethodPromotion(t *testing.T) {
	nodes := map[string]model.Node{
		"m": {ID: "m", Label: "Wrapper.Checksum", Kind: model.KindMethod, Language: "go", Recv: "b"},
	}
	fields := map[string]map[string]string{
		"Wrapper": {"S3Backend": "S3Backend", "name": "string"},
	}
	got := goReceiverQualified(model.Ref{FromID: "m", Name: "b.Get"}, nodes, fields, nil)
	want := []string{"Wrapper.Get", "S3Backend.Get"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate %d: got %q, want %q (own type must be tried first)", i, got[i], want[i])
		}
	}
	// A named field is not embedded and must not be a promotion candidate.
	if got := goReceiverQualified(model.Ref{FromID: "m", Name: "b.Missing"}, nodes, fields, nil); len(got) != 2 {
		t.Errorf("only embedded types promote: got %q", got)
	}
}

// TestGoStructFieldIndexDropsClashingNames: a struct name declared by two
// packages is dropped rather than guessed. Picking one would attach calls to
// whichever sorted first — a wrong edge carrying the same confidence as a
// right one.
func TestGoStructFieldIndexDropsClashingNames(t *testing.T) {
	nodes := map[string]model.Node{
		"a": {ID: "a", Label: "Config", Kind: model.KindStruct, Language: "go", File: "x/x.go", Fields: map[string]string{"db": "DBa"}},
		"b": {ID: "b", Label: "Config", Kind: model.KindStruct, Language: "go", File: "y/y.go", Fields: map[string]string{"db": "DBb"}},
		"c": {ID: "c", Label: "Server", Kind: model.KindStruct, Language: "go", File: "z/z.go", Fields: map[string]string{"e": "Engine"}},
	}
	idx := goStructFieldIndex(nodes)
	if _, ok := idx["Config"]; ok {
		t.Error("clashing struct name must be dropped, not guessed")
	}
	if idx["Server"]["e"] != "Engine" {
		t.Errorf("unambiguous struct lost: %v", idx["Server"])
	}
}

// TestBuildResolvesCallsThroughALocalVariable covers the other half of Go's
// receiver qualifiers: `eng := engine.New(); eng.Evaluate()`. No amount of
// struct knowledge reaches this shape — the receiver is a local, so the type
// comes from the declaration, and for the constructor idiom that means reading
// the callee's result type from another file.
func TestBuildResolvesCallsThroughALocalVariable(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/local\n\ngo 1.23\n")
	write(t, root, "engine/engine.go", `package engine

type Engine struct{}

func NewEngine() (*Engine, error) { return &Engine{}, nil }

func (e *Engine) Evaluate(in string) string { return in }
`)
	// A second Evaluate makes the bare tail ambiguous, so nothing below can
	// resolve by luck.
	write(t, root, "client/client.go", `package client

type Client struct{}

func (c *Client) Evaluate(in string) string { return in }
`)
	write(t, root, "app/app.go", `package app

import "example.com/local/engine"

// constructor result
func viaConstructor() string {
	eng, err := engine.NewEngine()
	if err != nil {
		return ""
	}
	return eng.Evaluate("x")
}

// composite literal
func viaLiteral() string {
	eng := &engine.Engine{}
	return eng.Evaluate("x")
}

// typed var
func viaVar() string {
	var eng *engine.Engine
	return eng.Evaluate("x")
}

// parameter
func viaParam(eng *engine.Engine) string {
	return eng.Evaluate("x")
}
`)
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
		t.Fatalf("Run: %v", err)
	}

	edges := edgeSet(t, s)
	for _, fn := range []string{"viaConstructor", "viaLiteral", "viaVar", "viaParam"} {
		want := "go:function:app/app.go#" + fn + " -calls-> go:method:engine/engine.go#Engine.Evaluate"
		if _, ok := edges[want]; !ok {
			t.Errorf("%s: local receiver did not resolve\nwant: %s", fn, want)
		}
		wrong := "go:function:app/app.go#" + fn + " -calls-> go:method:client/client.go#Client.Evaluate"
		if _, ok := edges[wrong]; ok {
			t.Errorf("%s: local scoping leaked onto an unrelated type", fn)
		}
	}
}

// TestGoResolveLocalsRefusesToGuess: a constructor whose name is ambiguous, or
// which returns nothing named, leaves the variable untyped rather than picking
// a candidate. An untyped local drops the call — the status quo — while a
// wrongly typed one manufactures a confident wrong edge.
func TestGoResolveLocalsRefusesToGuess(t *testing.T) {
	nodes := map[string]model.Node{
		"caller": {ID: "caller", Label: "run", Kind: model.KindFunction, Language: "go",
			Locals: []model.Local{
				{Name: "a", Type: "()NewThing", Start: 2, End: 9},
				{Name: "b", Type: "()NewDup", Start: 3, End: 9},
				{Name: "c", Type: "()NewVoid", Start: 4, End: 9},
				{Name: "d", Type: "Engine", Start: 5, End: 9},
			}},
		"nt":  {ID: "nt", Label: "NewThing", Kind: model.KindFunction, Language: "go", Returns: "Thing"},
		"nd1": {ID: "nd1", Label: "NewDup", Kind: model.KindFunction, Language: "go", Returns: "DupA"},
		"nd2": {ID: "nd2", Label: "NewDup", Kind: model.KindFunction, Language: "go", Returns: "DupB"},
		"nv":  {ID: "nv", Label: "NewVoid", Kind: model.KindFunction, Language: "go"},
	}
	symbols := map[string][]string{
		"go\x00NewThing": {"nt"},
		"go\x00NewDup":   {"nd1", "nd2"},
		"go\x00NewVoid":  {"nv"},
	}
	got := goResolveLocals(nodes, symbols)["caller"]
	at := func(name string) string { return localTypeAt(got, name, 6) }
	if at("a") != "Thing" {
		t.Errorf("unique constructor: got %q, want Thing", at("a"))
	}
	if at("b") != "" {
		t.Errorf("ambiguous constructor must not resolve: got %q", at("b"))
	}
	if at("c") != "" {
		t.Errorf("constructor returning nothing named must not resolve: got %q", at("c"))
	}
	if at("d") != "Engine" {
		t.Errorf("already-typed local lost: got %q", at("d"))
	}
}

// TestLocalTypeAtHonoursShadowing: Go lets an inner scope rebind a name to a
// different type. Flattening to "last declaration in source order wins"
// mistypes every call in the inner scope when an outer redeclaration follows
// it — and a mistyped receiver is a confident wrong edge, worse than a drop.
func TestLocalTypeAtHonoursShadowing(t *testing.T) {
	// func f() {            // 1
	//   c := NewOuter()     // 2   Outer, lines 2..12
	//   if x {              // 3
	//     c := NewInner()   // 4   Inner, lines 4..6
	//     c.Do()            // 5   -> Inner
	//   }                   // 6
	//   c.Do()              // 7   -> Outer
	// }                     // 12
	locals := []model.Local{
		{Name: "c", Type: "Outer", Start: 2, End: 12},
		{Name: "c", Type: "Inner", Start: 4, End: 6},
	}
	if got := localTypeAt(locals, "c", 5); got != "Inner" {
		t.Errorf("inside the inner scope: got %q, want Inner", got)
	}
	if got := localTypeAt(locals, "c", 7); got != "Outer" {
		t.Errorf("after the inner scope closes: got %q, want Outer", got)
	}
	if got := localTypeAt(locals, "c", 1); got != "" {
		t.Errorf("before any declaration: got %q, want none", got)
	}
	if got := localTypeAt(locals, "nope", 5); got != "" {
		t.Errorf("undeclared name: got %q, want none", got)
	}
	// Two declarations in the SAME scope: the later one wins from its line on.
	seq := []model.Local{
		{Name: "v", Type: "First", Start: 2, End: 10},
		{Name: "v", Type: "Second", Start: 5, End: 10},
	}
	if got := localTypeAt(seq, "v", 3); got != "First" {
		t.Errorf("before redeclaration: got %q, want First", got)
	}
	if got := localTypeAt(seq, "v", 6); got != "Second" {
		t.Errorf("after redeclaration: got %q, want Second", got)
	}
}

// TestBuildResolvesShadowedLocalsPerScope is the end-to-end proof: the same
// variable name bound to two different types in one function must resolve each
// call to the binding actually in scope at that line.
func TestBuildResolvesShadowedLocalsPerScope(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/shadow\n\ngo 1.23\n")
	write(t, root, "a/a.go", `package a

type Alpha struct{}

func NewAlpha() *Alpha { return &Alpha{} }

func (x *Alpha) Run() string { return "a" }
`)
	write(t, root, "b/b.go", `package b

type Beta struct{}

func NewBeta() *Beta { return &Beta{} }

func (x *Beta) Run() string { return "b" }
`)
	// `v` is an Alpha at the outer level and a Beta inside the if. The outer
	// call comes AFTER the inner one in source order, so a flat
	// last-declaration-wins map would type the inner call as Alpha.
	write(t, root, "app/app.go", `package app

import (
	"example.com/shadow/a"
	"example.com/shadow/b"
)

func Run(flag bool) string {
	v := a.NewAlpha()
	if flag {
		v := b.NewBeta()
		return v.Run()
	}
	return v.Run()
}
`)
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
		t.Fatalf("Run: %v", err)
	}
	edges := edgeSet(t, s)
	for _, want := range []string{
		"go:function:app/app.go#Run -calls-> go:method:a/a.go#Alpha.Run",
		"go:function:app/app.go#Run -calls-> go:method:b/b.go#Beta.Run",
	} {
		if _, ok := edges[want]; !ok {
			t.Errorf("missing edge for one of the two scopes\nwant: %s\nedges: %v", want, edges)
		}
	}
}
