package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphy/build"
	"okfw/cli/internal/graphy/export"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

// TestPipelineBuildThenExportProducesAValidBundle is the end-to-end test the
// suite was missing: export/okfw_test.go hand-builds stores from literal
// model.Node values and never calls build.Run, while build/build_test.go never
// calls export.ToOkfw. Nothing verified that *real extractor output* — real
// node ids, real degrees, real community labels — produces a bundle that
// passes OKF validation, which is the pipeline's central advertised guarantee.
func TestPipelineBuildThenExportProducesAValidBundle(t *testing.T) {
	root := pipelineRepo(t)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	bundle := initBundle(t, root)

	first := buildAndExport(t, w, bundle)
	if first.Created == 0 {
		t.Fatal("first export created nothing")
	}
	if first.Deleted != 0 || first.Updated != 0 {
		t.Errorf("first export should only create, got %+v", first)
	}

	// Every concept must be valid OKF, and must carry the graphy provenance
	// block that makes a re-export recoverable.
	concepts := conceptFiles(t, filepath.Join(bundle.RootPath, first.Dir))
	if len(concepts) == 0 {
		t.Fatal("no concept files written")
	}
	for _, path := range concepts {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		concept, err := config.ParseAndValidate(content)
		if err != nil {
			t.Errorf("%s is not a valid OKF concept: %v", filepath.Base(path), err)
			continue
		}
		if concept.Type == "" || !strings.HasPrefix(concept.Type, "code-") {
			t.Errorf("%s: type = %q, want a code-* type", filepath.Base(path), concept.Type)
		}
		if !strings.Contains(string(content), "graphy:") {
			t.Errorf("%s is missing the graphy provenance block", filepath.Base(path))
		}
	}

	// A second cycle over unchanged sources must be a no-op. This is the
	// guarantee that makes the bundle safe to commit and diff.
	second := buildAndExport(t, w, bundle)
	if second.Created != 0 || second.Updated != 0 || second.Deleted != 0 {
		t.Errorf("re-export of an unchanged repo should be a no-op, got %+v", second)
	}
	if second.Unchanged != first.Created {
		t.Errorf("re-export saw %d unchanged, want %d", second.Unchanged, first.Created)
	}
}

// TestPipelineDoesNotReingestItsOwnBundle is the end-to-end form of the
// self-ingestion bug: the bundle is tracked by git by design, so before the
// two listing modes shared one filter, the build after an export pulled the
// exported concepts back in as markdown heading nodes.
func TestPipelineDoesNotReingestItsOwnBundle(t *testing.T) {
	root := pipelineRepo(t)
	w, err := workspace.Init(root)
	if err != nil {
		t.Fatal(err)
	}
	bundle := initBundle(t, root)

	buildAndExport(t, w, bundle)

	// Rebuild from scratch now that the bundle exists on disk.
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := build.Run(w, s, build.Options{Full: true}); err != nil {
		t.Fatal(err)
	}

	if err := s.EachNode(func(n model.Node) error {
		if strings.Contains(n.File, ".okfw/") || strings.Contains(n.File, ".graphy/") {
			t.Errorf("graphy ingested its own output: %s (%s)", n.ID, n.File)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// pipelineRepo writes a repo with enough structure to exercise communities,
// rationale, and cross-file resolution end to end.
func pipelineRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/pipeline\n\ngo 1.23\n")
	write(t, root, "store/store.go", `package store

// WHY: reads outnumber writes, so the index is kept in memory.
func Get(k string) string { return lookup(k) }

func lookup(k string) string { return k }

func Put(k, v string) { record(k, v) }

func record(k, v string) {}
`)
	write(t, root, "api/api.go", `package api

import "example.com/pipeline/store"

func Handle(k string) string { return store.Get(k) }

func Save(k, v string) { store.Put(k, v) }
`)
	write(t, root, "main.go", `package main

import "example.com/pipeline/api"

func main() { api.Handle("k") }
`)
	return root
}

func initBundle(t *testing.T, root string) *config.Config {
	t.Helper()
	bundleRoot := filepath.Join(root, ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	return config.ConfigFor(bundleRoot)
}

// buildAndExport runs the real pipeline: extract, resolve, cluster, export.
func buildAndExport(t *testing.T, w *workspace.Workspace, bundle *config.Config) *export.OkfwResult {
	t.Helper()
	s, err := store.Open(w.DBPath())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := build.Run(w, s, build.Options{}); err != nil {
		t.Fatalf("build: %v", err)
	}
	res, err := export.ToOkfw(w.Root, s, export.OkfwOptions{
		Bundle:    bundle,
		MinDegree: 1,
		Generator: "graphy test",
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return res
}

func conceptFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") && d.Name() != "index.md" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
