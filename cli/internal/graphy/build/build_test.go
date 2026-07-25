package build

import (
	"os"
	"path/filepath"
	"testing"

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
