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
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Errorf("%s fails validation: %v", name, err)
			continue
		}
		if z.Graphy == nil || z.Graphy.NodeID == "" {
			t.Errorf("%s missing graphy extension block: %+v", name, z.Graphy)
		}
		text := string(content)
		if !strings.Contains(text, "<!-- graphy:node ") {
			t.Errorf("%s missing node marker:\n%s", name, text)
		}
		if strings.HasPrefix(name, "alpha--") {
			sawAlpha = true
			if z.Type != "code-function" {
				t.Errorf("Alpha type = %q, want code-function", z.Type)
			}
			if z.Resource != "a.go#L3" {
				t.Errorf("Alpha resource = %q, want a.go#L3", z.Resource)
			}
			if z.Graphy.NodeID != "go:function:a.go#Alpha" || z.Graphy.Entity != "function" || z.Graphy.Language != "go" {
				t.Errorf("Alpha graphy block = %+v", z.Graphy)
			}
			if !strings.Contains(text, "calls [Beta](./beta--") {
				t.Errorf("Alpha body lacks link to Beta:\n%s", text)
			}
			if z.Description != "Alpha does things." {
				t.Errorf("Alpha description = %q", z.Description)
			}
			if z.Parent == "" {
				t.Errorf("Alpha has no parent community:\n%s", text)
			}
		}
		if strings.HasPrefix(name, "community-core--") {
			sawCommunity = true
			if z.Type != "code-community" {
				t.Errorf("community type = %q, want code-community", z.Type)
			}
			if z.Graphy.NodeID != "community:0" || z.Graphy.Entity != "community" {
				t.Errorf("community graphy block = %+v", z.Graphy)
			}
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
	// Nothing changed → every file is skipped byte-for-byte (no timestamp churn).
	if res.Updated != 0 || res.Unchanged != len(first) {
		t.Errorf("re-export updated=%d unchanged=%d, want 0/%d", res.Updated, res.Unchanged, len(first))
	}
	second := conceptFiles(t, cfg, "graphy/myproj")
	if len(second) != len(first) {
		t.Fatalf("file count changed: %d → %d", len(first), len(second))
	}
	for name, content := range second {
		if string(first[name]) != string(content) {
			t.Errorf("%s bytes changed on no-op re-export", name)
		}
		c, err := config.ParseFrontmatter(content)
		if err != nil {
			t.Fatal(err)
		}
		if firstIDs[name] != c.ID {
			t.Errorf("%s concept id changed: %s → %s", name, firstIDs[name], c.ID)
		}
	}
}

func TestExportRewritesOnMeaningfulChange(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}

	// Alpha's doc changes — its concept (and only content that renders
	// differently) must be rewritten; the rest skip.
	nodes := []model.Node{
		{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Doc: "Alpha now does MORE things.",
			Signature: "func Alpha()", Degree: 4},
		{ID: "go:function:b.go#Beta", Label: "Beta", Kind: model.KindFunction, Language: "go",
			File: "b.go", Span: model.Span{StartLine: 8}, Degree: 3},
		{ID: "go:function:c.go#Tiny", Label: "Tiny", Kind: model.KindFunction, Language: "go",
			File: "c.go", Span: model.Span{StartLine: 1}, Degree: 1},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{nodes[0].ID: 0, nodes[1].ID: 0, nodes[2].ID: 0},
		map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 || res.Unchanged != 2 {
		t.Errorf("updated=%d unchanged=%d, want 1/2", res.Updated, res.Unchanged)
	}
	for name, content := range conceptFiles(t, cfg, "graphy/myproj") {
		if strings.HasPrefix(name, "alpha--") && !strings.Contains(string(content), "MORE things") {
			t.Errorf("Alpha concept not rewritten:\n%s", content)
		}
	}
}

func TestExportRenameKeepsConceptIdentity(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}
	var alphaID string
	for name, content := range conceptFiles(t, cfg, "graphy/myproj") {
		if strings.HasPrefix(name, "alpha--") {
			z, _ := config.ParseFrontmatter(content)
			alphaID = z.ID
		}
	}
	if alphaID == "" {
		t.Fatal("no alpha concept after first export")
	}

	// Alpha is renamed to AlphaPrime in the same file — same kind, 1:1.
	nodes := []model.Node{
		{ID: "go:function:a.go#AlphaPrime", Label: "AlphaPrime", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Doc: "Alpha does things.",
			Signature: "func AlphaPrime()", Degree: 4},
		{ID: "go:function:b.go#Beta", Label: "Beta", Kind: model.KindFunction, Language: "go",
			File: "b.go", Span: model.Span{StartLine: 8}, Degree: 3},
		{ID: "go:function:c.go#Tiny", Label: "Tiny", Kind: model.KindFunction, Language: "go",
			File: "c.go", Span: model.Span{StartLine: 1}, Degree: 1},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{nodes[0].ID: 0, nodes[1].ID: 0, nodes[2].ID: 0},
		map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Renamed != 1 || res.Created != 0 || res.Deleted != 0 {
		t.Errorf("renamed=%d created=%d deleted=%d, want 1/0/0", res.Renamed, res.Created, res.Deleted)
	}
	files := conceptFiles(t, cfg, "graphy/myproj")
	found := false
	for name, content := range files {
		if strings.HasPrefix(name, "alpha--") {
			t.Errorf("old alpha file survived: %s", name)
		}
		if strings.HasPrefix(name, "alphaprime--") {
			found = true
			z, err := config.ParseAndValidate(content)
			if err != nil {
				t.Fatalf("renamed concept invalid: %v", err)
			}
			if z.ID != alphaID {
				t.Errorf("concept id changed on rename: %s → %s", alphaID, z.ID)
			}
			if z.Title != "AlphaPrime" || z.Graphy.NodeID != "go:function:a.go#AlphaPrime" {
				t.Errorf("renamed concept = %q / %+v", z.Title, z.Graphy)
			}
		}
	}
	if !found {
		t.Errorf("no alphaprime concept: %v", keys(files))
	}
}

func TestExportMoveKeepsConceptIdentity(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}
	var betaID string
	for name, content := range conceptFiles(t, cfg, "graphy/myproj") {
		if strings.HasPrefix(name, "beta--") {
			z, _ := config.ParseFrontmatter(content)
			betaID = z.ID
		}
	}

	// Beta moves b.go → pkg/moved.go keeping its name.
	nodes := []model.Node{
		{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Doc: "Alpha does things.",
			Signature: "func Alpha()", Degree: 4},
		{ID: "go:function:pkg/moved.go#Beta", Label: "Beta", Kind: model.KindFunction, Language: "go",
			File: "pkg/moved.go", Span: model.Span{StartLine: 12}, Degree: 3},
		{ID: "go:function:c.go#Tiny", Label: "Tiny", Kind: model.KindFunction, Language: "go",
			File: "c.go", Span: model.Span{StartLine: 1}, Degree: 1},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{nodes[0].ID: 0, nodes[1].ID: 0, nodes[2].ID: 0},
		map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Renamed != 1 || res.Created != 0 || res.Deleted != 0 {
		t.Errorf("renamed=%d created=%d deleted=%d, want 1/0/0", res.Renamed, res.Created, res.Deleted)
	}
	for name, content := range conceptFiles(t, cfg, "graphy/myproj") {
		if !strings.HasPrefix(name, "beta--") {
			continue
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			t.Fatal(err)
		}
		if z.ID != betaID {
			t.Errorf("concept id changed on move: %s → %s", betaID, z.ID)
		}
		if z.Resource != "pkg/moved.go#L12" {
			t.Errorf("resource not updated after move: %q", z.Resource)
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

func TestExportRepoURLPermalink(t *testing.T) {
	s, cfg := fixture(t)
	// The store fixture lives outside any git repo, so commit is "" — pin one
	// by exporting from this actual repo root where git resolves HEAD.
	res, err := ToOkfw(mustGitRoot(t), s, OkfwOptions{
		Bundle:    cfg,
		MinDegree: 3,
		Project:   "myproj",
		RepoURL:   "https://github.com/example/myproj/",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range conceptFiles(t, cfg, res.Dir) {
		if !strings.HasPrefix(name, "alpha--") {
			continue
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(z.Resource, "https://github.com/example/myproj/blob/") ||
			!strings.HasSuffix(z.Resource, "/a.go#L3") {
			t.Errorf("resource = %q, want commit-pinned permalink", z.Resource)
		}
		if z.Graphy == nil || len(z.Graphy.Commit) < 7 {
			t.Errorf("graphy.commit missing: %+v", z.Graphy)
		}
	}
}

func mustGitRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Walk up to the repo root (contains .git).
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if filepath.Dir(dir) == dir {
			t.Skip("not inside a git repo")
		}
	}
}

func TestExportRecoversLostMapping(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}
	first := conceptFiles(t, cfg, "graphy/myproj")

	// Simulate a wiped .graphy/graph.db: drop every export record.
	var lost []string
	if err := s.EachExport(func(key string, _ store.ExportRecord) error {
		lost = append(lost, key)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, key := range lost {
		if err := s.DeleteExport(key); err != nil {
			t.Fatal(err)
		}
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 {
		t.Errorf("Created = %d, want 0 (concepts should be adopted, not duplicated)", res.Created)
	}
	if res.Recovered != len(lost) {
		t.Errorf("Recovered = %d, want %d", res.Recovered, len(lost))
	}
	second := conceptFiles(t, cfg, "graphy/myproj")
	if len(second) != len(first) {
		t.Errorf("file count changed after recovery: %d → %d (%v)", len(first), len(second), keys(second))
	}
	for name := range second {
		if _, ok := first[name]; !ok {
			t.Errorf("recovery minted a new file: %s", name)
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
