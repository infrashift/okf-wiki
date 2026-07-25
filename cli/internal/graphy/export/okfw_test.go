package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// fixture builds a store with one community and three nodes (two significant,
// one below the degree threshold), plus a temp bundle to export into.
func fixture(t *testing.T) (*store.Store, *config.Config) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Doc: "Alpha does things.",
			Signature: "func Alpha()", Degree: 4},
		{ID: "go:function:b.go#Beta", Label: "Beta", Kind: model.KindFunction, Language: "go",
			File: "b.go", Span: model.Span{StartLine: 8}, Degree: 3},
		{ID: "go:function:c.go#Tiny", Label: "Tiny", Kind: model.KindFunction, Language: "go",
			File: "c.go", Span: model.Span{StartLine: 1}, Degree: 1},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: nodes[0].ID, Target: nodes[1].ID, Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: nodes[1].ID, Target: nodes[2].ID, Relation: model.RelCalls, Confidence: model.ConfInferred},
	}); err != nil {
		t.Fatal(err)
	}
	assign := map[string]int{nodes[0].ID: 0, nodes[1].ID: 0, nodes[2].ID: 0}
	if err := s.SetCommunities(assign, map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}

	bundleRoot := filepath.Join(t.TempDir(), ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	return s, config.ConfigFor(bundleRoot)
}

func ids(nodes []model.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.ID
	}
	return out
}

func conceptFiles(t *testing.T, cfg *config.Config, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	root := filepath.Join(cfg.RootPath, filepath.FromSlash(dir))
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") && e.Name() != "index.md" {
			content, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			out[e.Name()] = content
		}
	}
	return out
}

func TestExportCreatesValidConcepts(t *testing.T) {
	s, cfg := fixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatalf("ToOkfw: %v", err)
	}
	// 1 community + Alpha + Beta (Tiny is below threshold).
	if res.Created != 3 {
		t.Errorf("Created = %d, want 3", res.Created)
	}

	files := conceptFiles(t, cfg, res.Dir)
	if len(files) != 3 {
		t.Fatalf("concept files = %d (%v), want 3", len(files), files)
	}
	var sawAlpha, sawCommunity bool
	for name, content := range files {
		// Every produced file must be schema-valid OKF.
		if _, err := config.ParseAndValidate(content); err != nil {
			t.Errorf("%s fails validation: %v", name, err)
		}
		text := string(content)
		if strings.HasPrefix(name, "alpha--") {
			sawAlpha = true
			if !strings.Contains(text, "calls [Beta](./beta--") {
				t.Errorf("Alpha body lacks link to Beta:\n%s", text)
			}
			if !strings.Contains(text, `description: "Alpha does things."`) {
				t.Errorf("Alpha description missing:\n%s", text)
			}
			if !strings.Contains(text, "parent:") {
				t.Errorf("Alpha has no parent community:\n%s", text)
			}
		}
		if strings.HasPrefix(name, "community-core--") {
			sawCommunity = true
			if !strings.Contains(text, "| [Alpha](./alpha--") {
				t.Errorf("community table lacks Alpha link:\n%s", text)
			}
			// Tiny is unexported → plain text, no link.
			if strings.Contains(text, "[Tiny](") {
				t.Errorf("community links unexported node:\n%s", text)
			}
		}
	}
	if !sawAlpha || !sawCommunity {
		t.Errorf("expected alpha + community files, got %v", keys(files))
	}

	// Bundle index.md files were maintained.
	if _, err := os.Stat(filepath.Join(cfg.RootPath, "index.md")); err != nil {
		t.Errorf("root index.md missing: %v", err)
	}
}

func TestExportIsIdempotent(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}
	first := conceptFiles(t, cfg, "graphy/myproj")
	firstIDs := map[string]string{}
	for name, content := range first {
		c, err := config.ParseFrontmatter(content)
		if err != nil {
			t.Fatal(err)
		}
		firstIDs[name] = c.ID
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Deleted != 0 {
		t.Errorf("re-export created=%d deleted=%d, want 0/0", res.Created, res.Deleted)
	}
	second := conceptFiles(t, cfg, "graphy/myproj")
	if len(second) != len(first) {
		t.Fatalf("file count changed: %d → %d", len(first), len(second))
	}
	for name, content := range second {
		c, err := config.ParseFrontmatter(content)
		if err != nil {
			t.Fatal(err)
		}
		if firstIDs[name] != c.ID {
			t.Errorf("%s concept id changed: %s → %s", name, firstIDs[name], c.ID)
		}
	}
}

func TestExportRemovesStaleConcepts(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}

	// Beta vanishes from the graph (e.g. renamed): re-put the file without it.
	// Alpha and Tiny remain, so the community itself survives.
	alpha := model.Node{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction,
		Language: "go", File: "a.go", Degree: 4}
	tiny := model.Node{ID: "go:function:c.go#Tiny", Label: "Tiny", Kind: model.KindFunction,
		Language: "go", File: "c.go", Degree: 1}
	remaining := []model.Node{alpha, tiny}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(remaining)}, remaining); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{alpha.ID: 0, tiny.ID: 0}, map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1 (Beta)", res.Deleted)
	}
	for name := range conceptFiles(t, cfg, "graphy/myproj") {
		if strings.HasPrefix(name, "beta--") {
			t.Errorf("stale Beta concept survived: %s", name)
		}
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
