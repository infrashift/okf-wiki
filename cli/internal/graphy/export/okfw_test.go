package export

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/query"
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
	// Graph overview + 1 community + Alpha + Beta (Tiny is below threshold).
	if res.Created != 4 {
		t.Errorf("Created = %d, want 4", res.Created)
	}

	files := conceptFiles(t, cfg, res.Dir)
	if len(files) != 4 {
		t.Fatalf("concept files = %d (%v), want 4", len(files), files)
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
			// Edges are grouped under a per-relation heading carrying the full
			// count, so a truncated list still says how much it stands for.
			if !strings.Contains(text, "### calls (1)") || !strings.Contains(text, "- [Beta](./beta--") {
				t.Errorf("Alpha body lacks the grouped link to Beta:\n%s", text)
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
			// Keyed by label slug, not by the detection id: ids renumber
			// whenever the partition shifts.
			if z.Graphy.NodeID != "community:core" || z.Graphy.Entity != "community" {
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
	// Only Alpha's doc changed; the overview, the community and Beta are
	// byte-identical and must not be rewritten.
	if res.Updated != 1 || res.Unchanged != 3 {
		t.Errorf("updated=%d unchanged=%d, want 1/3", res.Updated, res.Unchanged)
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

// rationaleFixture adds two rationale nodes to the standard fixture: a WHY on
// the exported Alpha, and a TODO on the below-threshold Tiny.
func rationaleFixture(t *testing.T) (*store.Store, *config.Config) {
	t.Helper()
	s, cfg := fixture(t)

	why := model.Node{
		ID:    model.NodeID("go", model.KindRationale, "a.go", model.RationaleQual("Alpha", "WHY", 0)),
		Label: "WHY: callers depend on the ordering", Kind: model.KindRationale, Language: "go",
		File: "a.go", Span: model.Span{StartLine: 1},
		Doc:    "callers depend on the ordering",
		Body:   "callers depend on the ordering\nso this must not be reordered.",
		Degree: 2,
	}
	todo := model.Node{
		ID:    model.NodeID("go", model.KindRationale, "c.go", model.RationaleQual("Tiny", "TODO", 0)),
		Label: "TODO: delete once the migration lands", Kind: model.KindRationale, Language: "go",
		File: "c.go", Span: model.Span{StartLine: 1},
		Doc: "delete once the migration lands", Degree: 2,
	}
	extra := []model.Node{why, todo}
	if err := s.PutFile(store.FileRecord{Path: "rationale.go", NodeIDs: ids(extra)}, extra); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: "go:function:a.go#Alpha", Target: "go:function:b.go#Beta",
			Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: why.ID, Target: "go:function:a.go#Alpha",
			Relation: model.RelExplains, Confidence: model.ConfExtracted},
		{Source: todo.ID, Target: "go:function:c.go#Tiny",
			Relation: model.RelExplains, Confidence: model.ConfExtracted},
	}); err != nil {
		t.Fatal(err)
	}
	assign := map[string]int{
		"go:function:a.go#Alpha": 0, "go:function:b.go#Beta": 0, "go:function:c.go#Tiny": 0,
		why.ID: 0, todo.ID: 0,
	}
	if err := s.SetCommunities(assign, map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}
	return s, cfg
}

// findByNodeID returns the concept exported for a graph node. Matching on the
// frontmatter node id rather than on body text matters: a rationale's prose
// also appears in its owner's edge list, so a substring search would find the
// wrong file.
func findByNodeID(t *testing.T, files map[string][]byte, nodeID string) (string, *config.Concept, string) {
	t.Helper()
	for name, content := range files {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		if z.Graphy != nil && z.Graphy.NodeID == nodeID {
			return name, z, string(content)
		}
	}
	return "", nil, ""
}

// findByType returns the single concept of a given OKF type.
func findByType(t *testing.T, files map[string][]byte, typ string) (string, *config.Concept, string) {
	t.Helper()
	for name, content := range files {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		if z.Type == typ {
			return name, z, string(content)
		}
	}
	return "", nil, ""
}

// rationale node ids used across the export tests.
var (
	whyNodeID  = model.NodeID("go", model.KindRationale, "a.go", model.RationaleQual("Alpha", "WHY", 0))
	todoNodeID = model.NodeID("go", model.KindRationale, "c.go", model.RationaleQual("Tiny", "TODO", 0))
)

// TestExportRationaleExemptFromMinDegree: degree is a significance proxy for
// code entities and meaningless for rationale, so the threshold must not apply.
func TestExportRationaleExemptFromMinDegree(t *testing.T) {
	s, cfg := rationaleFixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{
		Bundle: cfg, MinDegree: 3, RationaleMarkers: []string{"WHY", "TODO"},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, res.Dir)

	// Both degree-2 rationales are exported...
	if _, z, _ := findByNodeID(t, files, whyNodeID); z == nil {
		t.Error("degree-2 WHY rationale was dropped by the min-degree filter")
	} else if z.Type != "code-rationale" {
		t.Errorf("rationale type = %q, want code-rationale", z.Type)
	}
	if _, z, _ := findByNodeID(t, files, todoNodeID); z == nil {
		t.Error("degree-2 TODO rationale was dropped by the min-degree filter")
	}
	// ...while the degree-1 code entity it hangs off is still excluded.
	if name, _, _ := findByNodeID(t, files, "go:function:c.go#Tiny"); name != "" {
		t.Errorf("degree-1 function Tiny was exported as %s; min-degree should still apply to code", name)
	}
}

func TestExportRationaleMarkerFilter(t *testing.T) {
	s, cfg := rationaleFixture(t)
	// The default set excludes TODO deliberately: a repo with 800 TODOs should
	// not dump 800 concepts into the wiki.
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, res.Dir)
	if _, z, _ := findByNodeID(t, files, whyNodeID); z == nil {
		t.Error("WHY is in the default marker set but was not exported")
	}
	if name, _, _ := findByNodeID(t, files, todoNodeID); name != "" {
		t.Errorf("TODO is not in the default marker set but was exported as %s", name)
	}
}

func TestExportRationaleParentsToItsOwner(t *testing.T) {
	s, cfg := rationaleFixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{
		Bundle: cfg, MinDegree: 3, RationaleMarkers: []string{"WHY", "TODO"},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, res.Dir)

	_, alpha, _ := findByNodeID(t, files, "go:function:a.go#Alpha")
	if alpha == nil {
		t.Fatal("Alpha concept missing")
	}
	_, community, _ := findByType(t, files, "code-community")
	if community == nil {
		t.Fatal("community concept missing")
	}

	// Alpha was exported, so its WHY parents to Alpha.
	if _, why, _ := findByNodeID(t, files, whyNodeID); why == nil {
		t.Fatal("WHY concept missing")
	} else if why.Parent != alpha.ID {
		t.Errorf("WHY parent = %q, want Alpha's id %q", why.Parent, alpha.ID)
	}
	// Tiny was NOT exported, so its TODO falls back to the community parent
	// rather than dangling (a dangling parent fails the CUE schema).
	if _, todo, _ := findByNodeID(t, files, todoNodeID); todo == nil {
		t.Fatal("TODO concept missing")
	} else if todo.Parent != community.ID {
		t.Errorf("TODO parent = %q, want the community id %q", todo.Parent, community.ID)
	}
}

func TestExportRationaleBodyAndTags(t *testing.T) {
	s, cfg := rationaleFixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, res.Dir)
	_, z, text := findByNodeID(t, files, whyNodeID)
	if z == nil {
		t.Fatal("WHY concept missing")
	}
	if !strings.Contains(text, "so this must not be reordered.") {
		t.Error("multi-line rationale Body is missing from the concept body")
	}
	if !strings.Contains(text, "- Marker: WHY") {
		t.Error("marker bullet missing")
	}
	if strings.Contains(text, "- Degree:") {
		t.Error("rationale concepts must omit Degree: it is meaningless and would churn the file")
	}
	var sawTag bool
	for _, tag := range z.Tags {
		if tag == "graphy/rationale/why" {
			sawTag = true
		}
	}
	if !sawTag {
		t.Errorf("missing per-marker tag, got %v", z.Tags)
	}
	if z.Graphy == nil || z.Graphy.Entity != "rationale" {
		t.Errorf("graphy block = %+v, want entity rationale", z.Graphy)
	}
}

// TestReexportRationaleUnchanged is the idempotency guarantee: a no-op
// re-export must write nothing at all.
func TestReexportRationaleUnchanged(t *testing.T) {
	s, cfg := rationaleFixture(t)
	opts := OkfwOptions{Bundle: cfg, MinDegree: 3, RationaleMarkers: []string{"WHY", "TODO"}}
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.Created == 0 {
		t.Fatal("first export created nothing")
	}
	before := conceptFiles(t, cfg, first.Dir)

	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Updated != 0 || second.Deleted != 0 {
		t.Errorf("re-export was not a no-op: %+v", second)
	}
	if second.Unchanged != len(before) {
		t.Errorf("Unchanged = %d, want %d", second.Unchanged, len(before))
	}
	after := conceptFiles(t, cfg, second.Dir)
	for name, content := range before {
		if string(after[name]) != string(content) {
			t.Errorf("%s changed on a no-op re-export", name)
		}
	}
}

// TestExportRationaleTitleIsTruncated: bundle.Slugify has no length cap and
// NoteFilename concatenates the whole slug, so an untruncated prose line would
// produce a filename past the 255-byte limit.
func TestExportRationaleTitleIsTruncated(t *testing.T) {
	s, cfg := fixture(t)
	long := strings.TrimSpace(strings.Repeat("a very long winded explanation ", 20))
	n := model.Node{
		ID:    model.NodeID("go", model.KindRationale, "a.go", model.RationaleQual("Alpha", "WHY", 0)),
		Label: "WHY: " + long, Kind: model.KindRationale, Language: "go",
		File: "a.go", Span: model.Span{StartLine: 1}, Doc: long, Degree: 2,
	}
	if err := s.PutFile(store.FileRecord{Path: "r.go", NodeIDs: []string{n.ID}}, []model.Node{n}); err != nil {
		t.Fatal(err)
	}
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	for name := range conceptFiles(t, cfg, res.Dir) {
		if len(name) > 255 {
			t.Errorf("filename is %d bytes, past the filesystem limit: %s", len(name), name)
		}
	}
}

// TestAdoptRenamesSkipsRationaleInFile guards a real regression: rule 2 pairs
// 1:1 within (lang, kind, file), which for rationale would match a deleted WHY
// against a newly added TODO and carry a stale identity into unrelated
// reasoning.
func TestAdoptRenamesSkipsRationaleInFile(t *testing.T) {
	s, cfg := fixture(t)
	opts := OkfwOptions{Bundle: cfg, MinDegree: 3, RationaleMarkers: []string{"WHY", "TODO"}}

	why := model.Node{
		ID:    model.NodeID("go", model.KindRationale, "a.go", model.RationaleQual("Alpha", "WHY", 0)),
		Label: "WHY: the original reason", Kind: model.KindRationale, Language: "go",
		File: "a.go", Span: model.Span{StartLine: 1}, Doc: "the original reason", Degree: 2,
	}
	if err := s.PutFile(store.FileRecord{Path: "r.go", NodeIDs: []string{why.ID}}, []model.Node{why}); err != nil {
		t.Fatal(err)
	}
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	whyRec, err := s.GetExport(why.ID)
	if err != nil || whyRec == nil {
		t.Fatalf("WHY was not exported: %v", err)
	}

	// Replace the WHY with an unrelated TODO in the same file.
	todo := model.Node{
		ID:    model.NodeID("go", model.KindRationale, "a.go", model.RationaleQual("Alpha", "TODO", 0)),
		Label: "TODO: something entirely different", Kind: model.KindRationale, Language: "go",
		File: "a.go", Span: model.Span{StartLine: 1}, Doc: "something entirely different", Degree: 2,
	}
	if err := s.PutFile(store.FileRecord{Path: "r.go", NodeIDs: []string{todo.ID}}, []model.Node{todo}); err != nil {
		t.Fatal(err)
	}
	res, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Renamed != 0 {
		t.Errorf("Renamed = %d: a deleted WHY was adopted by an unrelated TODO", res.Renamed)
	}
	todoRec, err := s.GetExport(todo.ID)
	if err != nil || todoRec == nil {
		t.Fatal("TODO was not exported")
	}
	if todoRec.ConceptID == whyRec.ConceptID {
		t.Error("the TODO inherited the WHY's concept id")
	}
}

// conceptIDsByTitle maps each exported concept's title to its OKF id.
func conceptIDsByTitle(t *testing.T, files map[string][]byte) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, content := range files {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		out[z.Title] = z.ID
	}
	return out
}

// TestExportCommunitySurvivesRenumbering is the point of keying communities by
// label slug. Community ids are ordered by each community's smallest member, so
// adding or removing one node renumbers most of the graph. Under integer keys
// that deleted and recreated every affected concept, losing its id and
// repointing `parent:` on all of its children.
func TestExportCommunitySurvivesRenumbering(t *testing.T) {
	s, cfg := fixture(t)
	opts := OkfwOptions{Bundle: cfg, MinDegree: 3}

	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	before := conceptIDsByTitle(t, conceptFiles(t, cfg, first.Dir))
	communityID := before["Community: core"]
	alphaID := before["Alpha"]
	if communityID == "" || alphaID == "" {
		t.Fatalf("expected a community and Alpha concept, got %v", before)
	}

	// Renumber: same members, same label, different detection id.
	if err := s.SetCommunities(map[string]int{
		"go:function:a.go#Alpha": 7,
		"go:function:b.go#Beta":  7,
		"go:function:c.go#Tiny":  7,
	}, map[int]string{7: "core"}); err != nil {
		t.Fatal(err)
	}

	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Deleted != 0 {
		t.Errorf("renumbering churned the bundle: %+v", second)
	}
	after := conceptIDsByTitle(t, conceptFiles(t, cfg, second.Dir))
	if after["Community: core"] != communityID {
		t.Errorf("community concept id changed on renumbering: %q → %q",
			communityID, after["Community: core"])
	}
	if after["Alpha"] != alphaID {
		t.Errorf("child concept id changed: %q → %q", alphaID, after["Alpha"])
	}
}

// TestExportMigratesLegacyCommunityKeys covers bundles exported before
// label-slug keying, whose records are still filed under the integer id.
func TestExportMigratesLegacyCommunityKeys(t *testing.T) {
	s, cfg := fixture(t)
	opts := OkfwOptions{Bundle: cfg, MinDegree: 3}

	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, first.Dir)
	before := conceptIDsByTitle(t, files)
	communityID := before["Community: core"]

	// Rewrite the export record under the old integer-keyed scheme.
	newKey := communityKey("core")
	beforeRec, err := s.GetExport(newKey)
	if err != nil || beforeRec == nil {
		t.Fatalf("community export record missing: %v", err)
	}
	beforePath := beforeRec.RelPath
	rec, err := s.GetExport(newKey)
	if err != nil || rec == nil {
		t.Fatalf("community export record missing: %v", err)
	}
	if err := s.DeleteExport(newKey); err != nil {
		t.Fatal(err)
	}
	if err := s.PutExport(communityKeyPrefix+"0", *rec); err != nil {
		t.Fatal(err)
	}

	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Deleted != 0 {
		t.Errorf("legacy key was not migrated in place: %+v", second)
	}
	if second.Rekeyed != 1 {
		t.Errorf("Rekeyed = %d, want 1 (the re-keyed community)", second.Rekeyed)
	}
	after := conceptIDsByTitle(t, conceptFiles(t, cfg, second.Dir))
	if after["Community: core"] != communityID {
		t.Errorf("community id changed during migration: %q → %q",
			communityID, after["Community: core"])
	}
	// The old record must be gone, or the next stale sweep would delete the
	// concept it points at.
	if old, err := s.GetExport(communityKeyPrefix + "0"); err != nil || old != nil {
		t.Errorf("legacy export record survived migration: %+v", old)
	}
	got, err := s.GetExport(newKey)
	if err != nil || got == nil {
		t.Fatalf("new key not written: %v", err)
	}
	// "Re-keyed in place" has to mean the file too. The title is identical by
	// construction here, so renameToTitle must recognise the current filename as
	// already encoding it — the guard it replaced compared against
	// bundle.NoteFilename, which skips names that exist on disk and so always
	// proposed a longer shortid, renaming every migrated community.
	if got.RelPath != beforePath {
		t.Errorf("concept file renamed during an in-place re-key: %q → %q", beforePath, got.RelPath)
	}
}

// TestExportCommunitySplitDoesNotStealALivePlacement pins the ordering that
// makes adoption safe: a record backing a community resolved from its own live
// export record must be off limits to every other community's adoption.
//
// Here "alpha" keeps its label — and so its key, and so its live record — while
// shedding most of its members to a new "beta". beta's membership still overlaps
// alpha's *recorded* membership by two thirds, so the overlap matcher used to
// hand beta the record alpha was already placed on: it renamed alpha's file and
// deleted alpha's record, and the render pass then died reading a path that no
// longer existed, after both writes had landed.
func TestExportCommunitySplitDoesNotStealALivePlacement(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3, 4, 5}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	alphaKey := communityKey("alpha")
	before, err := s.GetExport(alphaKey)
	if err != nil || before == nil {
		t.Fatalf("alpha community record missing: %v", err)
	}

	// beta takes {2,3,4,5}: 4 of the 6 alpha used to hold, over a union of 6 —
	// 0.667, comfortably past the threshold.
	partition(t, s, map[string][]int{"alpha": {0, 1}, "beta": {2, 3, 4, 5}})
	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatalf("export after community split: %v", err)
	}

	after, err := s.GetExport(alphaKey)
	if err != nil {
		t.Fatal(err)
	}
	if after == nil {
		t.Fatal("alpha's live record was consumed by beta's adoption")
	}
	if after.RelPath != before.RelPath {
		t.Errorf("alpha's concept was renamed out from under its live placement: %q → %q",
			before.RelPath, after.RelPath)
	}
	if _, err := os.Stat(filepath.Join(cfg.RootPath, filepath.FromSlash(after.RelPath))); err != nil {
		t.Errorf("alpha's concept file is gone: %v", err)
	}
	titles := communityConcepts(t, cfg, second.Dir)
	if titles["Community: alpha"] == "" || titles["Community: beta"] == "" {
		t.Errorf("want both communities present, got %v", titles)
	}
	if second.Rekeyed != 0 || second.Created != 1 {
		t.Errorf("beta should have been created fresh, not adopted: %+v", second)
	}
}

func TestUniqueSlugDisambiguatesCollidingLabels(t *testing.T) {
	used := map[string]bool{}
	got := []string{
		uniqueSlug("store", used),
		uniqueSlug("store", used),
		uniqueSlug("store", used),
		uniqueSlug("other", used),
	}
	want := []string{"store", "store-2", "store-3", "other"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("uniqueSlug sequence = %v, want %v", got, want)
		}
	}
}

// TestExportCollidingCommunityLabels: two communities can name themselves after
// same-named members in different packages, and both must still get a concept.
func TestExportCollidingCommunityLabels(t *testing.T) {
	s, cfg := fixture(t)
	// Split the three nodes into two communities that share a label.
	if err := s.SetCommunities(map[string]int{
		"go:function:a.go#Alpha": 0,
		"go:function:b.go#Beta":  0,
		"go:function:c.go#Tiny":  1,
	}, map[int]string{0: "store", 1: "store"}); err != nil {
		t.Fatal(err)
	}
	// A second member for community 1, so it clears the two-member threshold.
	extra := model.Node{ID: "go:function:d.go#Delta", Label: "Delta", Kind: model.KindFunction,
		Language: "go", File: "d.go", Span: model.Span{StartLine: 1}, Degree: 3}
	if err := s.PutFile(store.FileRecord{Path: "d.go", NodeIDs: []string{extra.ID}}, []model.Node{extra}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(map[string]int{
		"go:function:a.go#Alpha": 0,
		"go:function:b.go#Beta":  0,
		"go:function:c.go#Tiny":  1,
		extra.ID:                 1,
	}, map[int]string{0: "store", 1: "store"}); err != nil {
		t.Fatal(err)
	}

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	files := conceptFiles(t, cfg, res.Dir)
	rids := map[string]bool{}
	for name, content := range files {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		if z.Type == "code-community" {
			rids[z.Graphy.NodeID] = true
		}
	}
	if !rids["community:store"] || !rids["community:store-2"] {
		t.Errorf("colliding labels did not get distinct keys: %v", rids)
	}
}

// TestExportMigratesCollidingLegacyCommunities: community titles are not
// unique, so the legacy index must hand out one record per same-titled
// community rather than letting the first claim it and the rest create
// duplicates (which would also strand the unclaimed records for the stale
// sweep to delete).
func TestExportMigratesCollidingLegacyCommunities(t *testing.T) {
	s, cfg := fixture(t)
	extra := model.Node{ID: "go:function:d.go#Delta", Label: "Delta", Kind: model.KindFunction,
		Language: "go", File: "d.go", Span: model.Span{StartLine: 1}, Degree: 3}
	if err := s.PutFile(store.FileRecord{Path: "d.go", NodeIDs: []string{extra.ID}}, []model.Node{extra}); err != nil {
		t.Fatal(err)
	}
	// Two communities, same label.
	assign := map[string]int{
		"go:function:a.go#Alpha": 0, "go:function:b.go#Beta": 0,
		"go:function:c.go#Tiny": 1, extra.ID: 1,
	}
	if err := s.SetCommunities(assign, map[int]string{0: "store", 1: "store"}); err != nil {
		t.Fatal(err)
	}
	opts := OkfwOptions{Bundle: cfg, MinDegree: 3}
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	before := conceptFiles(t, cfg, first.Dir)

	// Re-file both under the old integer-keyed scheme.
	for slug, oldID := range map[string]string{"store": "0", "store-2": "1"} {
		rec, err := s.GetExport(communityKey(slug))
		if err != nil || rec == nil {
			t.Fatalf("missing record for %s: %v", slug, err)
		}
		if err := s.DeleteExport(communityKey(slug)); err != nil {
			t.Fatal(err)
		}
		if err := s.PutExport(communityKeyPrefix+oldID, *rec); err != nil {
			t.Fatal(err)
		}
	}

	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Deleted != 0 {
		t.Errorf("colliding legacy communities churned: %+v", second)
	}
	if second.Rekeyed != 2 {
		t.Errorf("Rekeyed = %d, want 2 (both same-titled communities)", second.Rekeyed)
	}
	if after := conceptFiles(t, cfg, second.Dir); len(after) != len(before) {
		t.Errorf("file count changed: %d → %d", len(before), len(after))
	}
	for _, slug := range []string{"store", "store-2"} {
		if rec, err := s.GetExport(communityKey(slug)); err != nil || rec == nil {
			t.Errorf("no record under new key %q", slug)
		}
	}
	for _, oldID := range []string{"0", "1"} {
		if rec, err := s.GetExport(communityKeyPrefix + oldID); err != nil || rec != nil {
			t.Errorf("legacy record %q survived migration", oldID)
		}
	}
}

// --- community identity across relabelling -------------------------------

// partitionFixture builds a store of n plain nodes plus an empty bundle. Tests
// then declare partitions directly, which is the only variable that matters
// for community identity.
func partitionFixture(t *testing.T, n int) (*store.Store, *config.Config) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	var nodes []model.Node
	for i := 0; i < n; i++ {
		nodes = append(nodes, model.Node{
			ID: fmt.Sprintf("go:function:f.go#N%d", i), Label: fmt.Sprintf("N%d", i),
			Kind: model.KindFunction, Language: "go", File: "f.go",
			Span: model.Span{StartLine: i + 1}, Degree: 2,
		})
	}
	if err := s.PutFile(store.FileRecord{Path: "f.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(t.TempDir(), ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	return s, config.ConfigFor(bundleRoot)
}

func node(i int) string { return fmt.Sprintf("go:function:f.go#N%d", i) }

// partition assigns the listed node indices to communities and labels them.
func partition(t *testing.T, s *store.Store, groups map[string][]int) {
	t.Helper()
	assign := map[string]int{}
	labels := map[int]string{}
	// Sort labels so community ids are assigned deterministically.
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for id, name := range names {
		labels[id] = name
		for _, i := range groups[name] {
			assign[node(i)] = id
		}
	}
	if err := s.SetCommunities(assign, labels); err != nil {
		t.Fatal(err)
	}
}

// communityConcepts maps each exported community concept's title to its id.
func communityConcepts(t *testing.T, cfg *config.Config, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, content := range conceptFiles(t, cfg, dir) {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		if z.Type == "code-community" {
			out[z.Title] = z.ID
		}
	}
	return out
}

// TestExportCommunitySurvivesLabelChange is the point of membership matching:
// a community's label is its most prominent member's name, which changes as
// the code does — but the subsystem, and so the concept, is the same.
func TestExportCommunitySurvivesLabelChange(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3}})
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	before := communityConcepts(t, cfg, first.Dir)
	id := before["Community: alpha"]
	if id == "" {
		t.Fatalf("no community concept: %v", before)
	}

	// Same members, different label.
	partition(t, s, map[string][]int{"renamed": {0, 1, 2, 3}})
	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Deleted != 0 {
		t.Errorf("relabelling churned the bundle: %+v", second)
	}
	if second.Rekeyed != 1 {
		t.Errorf("Rekeyed = %d, want 1 (adopted via membership)", second.Rekeyed)
	}
	after := communityConcepts(t, cfg, second.Dir)
	if after["Community: renamed"] != id {
		t.Errorf("concept id changed on relabelling: %q → %q", id, after["Community: renamed"])
	}
	if _, stale := after["Community: alpha"]; stale {
		t.Error("the old title is still present; the concept was duplicated")
	}
	// The filename must follow the new title, or it describes what the concept
	// used to be.
	for name := range conceptFiles(t, cfg, second.Dir) {
		if strings.HasPrefix(name, "community-alpha--") {
			t.Errorf("filename %q still reflects the old label", name)
		}
	}
}

// TestExportCommunityNotAdoptedWhenMembershipReplaced: a different set of
// nodes under a different name is a different subsystem, whatever it is called.
func TestExportCommunityNotAdoptedWhenMembershipReplaced(t *testing.T) {
	s, cfg := partitionFixture(t, 8)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	partition(t, s, map[string][]int{"beta": {4, 5, 6, 7}})
	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Rekeyed != 0 {
		t.Errorf("Rekeyed = %d: a disjoint community must not adopt", second.Rekeyed)
	}
	if second.Created != 1 || second.Deleted != 1 {
		t.Errorf("want 1 created + 1 deleted, got %+v", second)
	}
}

// TestExportCommunitySplitEvenly: neither half is "the same subsystem", and
// the half-overlap rule makes that unambiguous rather than arbitrary.
func TestExportCommunitySplitEvenly(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	partition(t, s, map[string][]int{"left": {0, 1}, "right": {2, 3}})
	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Rekeyed != 0 {
		t.Errorf("Rekeyed = %d: an even split has no majority successor", second.Rekeyed)
	}
	if second.Created != 2 || second.Deleted != 1 {
		t.Errorf("want 2 created + 1 deleted, got %+v", second)
	}
}

// TestExportCommunitySplitUnevenly: the majority successor keeps the concept,
// and — critically — the minority one cannot also claim it.
func TestExportCommunitySplitUnevenly(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3}})
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	id := communityConcepts(t, cfg, first.Dir)["Community: alpha"]

	// {0,1,2} keeps 3 of 4 (0.75); {3,4} shares 1 of 5 (0.2).
	partition(t, s, map[string][]int{"major": {0, 1, 2}, "minor": {3, 4}})
	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Rekeyed != 1 {
		t.Errorf("Rekeyed = %d, want exactly 1", second.Rekeyed)
	}
	if second.Created != 1 || second.Deleted != 0 {
		t.Errorf("want 1 created + 0 deleted, got %+v", second)
	}
	after := communityConcepts(t, cfg, second.Dir)
	if after["Community: major"] != id {
		t.Errorf("majority successor did not inherit: %q → %q", id, after["Community: major"])
	}
	if after["Community: minor"] == id {
		t.Error("minority successor also claimed the concept")
	}
}

// TestExportCommunityMembershipRefreshedOnEveryExport guards the subtle part:
// the live-record path skips the write, so without an explicit refresh the
// stored membership would stay frozen at whatever it was when the key was
// last created — and the relabelling after that would not be recognized.
func TestExportCommunityMembershipRefreshedOnEveryExport(t *testing.T) {
	s, cfg := partitionFixture(t, 8)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}

	partition(t, s, map[string][]int{"alpha": {0, 1}})
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	id := communityConcepts(t, cfg, first.Dir)["Community: alpha"]

	// Grow under the same label — the key is unchanged, so this run takes the
	// live-record path and writes nothing but the refresh.
	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3, 4, 5}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	rec, err := s.GetExport(communityKey("alpha"))
	if err != nil || rec == nil {
		t.Fatal("community record missing")
	}
	if got := len(digestSet(rec.MemberDigest)); got != 6 {
		t.Fatalf("stored membership = %d nodes, want the current 6", got)
	}

	// Now relabel. Matching against the refreshed membership finds it; against
	// the stale two-node membership it would score 2/6 and fail.
	partition(t, s, map[string][]int{"renamed": {0, 1, 2, 3, 4, 5}})
	third, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if third.Rekeyed != 1 || third.Created != 0 || third.Deleted != 0 {
		t.Errorf("relabelling after growth churned: %+v", third)
	}
	if communityConcepts(t, cfg, third.Dir)["Community: renamed"] != id {
		t.Error("concept id changed")
	}
}

func TestJaccardDigest(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []string
		want float64
	}{
		{"identical", []string{"a", "b"}, []string{"a", "b"}, 1},
		{"disjoint", []string{"a", "b"}, []string{"c", "d"}, 0},
		{"one of three", []string{"a", "b"}, []string{"b", "c"}, 1.0 / 3},
		{"subset", []string{"a", "b", "c"}, []string{"a", "b", "c", "d"}, 0.75},
		{"empty left", nil, []string{"a"}, 0},
		{"empty right", []string{"a"}, nil, 0},
		{"repeated id counts once", []string{"a", "a", "b"}, []string{"a", "b"}, 1},
	} {
		got := jaccardDigest(memberDigest(tc.a), digestSet(memberDigest(tc.b)))
		if got != tc.want {
			t.Errorf("%s: jaccardDigest = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestMemberDigestIsCanonical(t *testing.T) {
	// Order and duplicates must not change the blob: an unchanged membership
	// has to compare equal byte for byte, or the write-skip never fires.
	a := memberDigest([]string{"n3", "n1", "n2"})
	b := memberDigest([]string{"n2", "n3", "n1", "n2"})
	if !bytes.Equal(a, b) {
		t.Errorf("digest depends on order or duplicates: %x vs %x", a, b)
	}
	if len(a) != 3*8 {
		t.Errorf("digest = %d bytes, want 3 packed hashes", len(a))
	}
	if bytes.Equal(a, memberDigest([]string{"n1", "n2", "n4"})) {
		t.Error("different memberships produced the same digest")
	}
	if len(memberDigest(nil)) != 0 {
		t.Error("empty membership should produce an empty digest")
	}
	// Round-trips through the set decoder.
	if got := len(digestSet(a)); got != 3 {
		t.Errorf("digestSet = %d entries, want 3", got)
	}
}

// TestExportBackfillsLegacyEmptyDigest: a community record written before
// membership digests existed decodes with an empty one (see store.ExportRecord).
// The next export must fill it in without touching the concept — an empty
// digest that stayed empty would leave the community unable to survive its
// next relabelling.
func TestExportBackfillsLegacyEmptyDigest(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}
	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3}})
	first, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	id := communityConcepts(t, cfg, first.Dir)["Community: alpha"]

	// Strip the digest, leaving the record as an older build would have.
	key := communityKey("alpha")
	rec, err := s.GetExport(key)
	if err != nil || rec == nil {
		t.Fatalf("community record missing: %v", err)
	}
	rec.MemberDigest = nil
	if err := s.PutExport(key, *rec); err != nil {
		t.Fatal(err)
	}

	second, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created != 0 || second.Deleted != 0 {
		t.Errorf("legacy record churned the bundle: %+v", second)
	}
	back, err := s.GetExport(key)
	if err != nil || back == nil {
		t.Fatalf("record missing after backfill: %v", err)
	}
	if len(back.MemberDigest) != 4*8 {
		t.Fatalf("digest = %d bytes, want 4 packed hashes", len(back.MemberDigest))
	}

	// And the backfilled digest is usable: the community survives a relabel.
	partition(t, s, map[string][]int{"renamed": {0, 1, 2, 3}})
	third, err := ToOkfw("/repo/myproj", s, opts)
	if err != nil {
		t.Fatal(err)
	}
	if third.Rekeyed != 1 {
		t.Errorf("Rekeyed = %d, want 1 (adopted via the backfilled digest)", third.Rekeyed)
	}
	if got := communityConcepts(t, cfg, third.Dir)["Community: renamed"]; got != id {
		t.Errorf("concept id changed after relabel: %q → %q", id, got)
	}
}

// TestExportSkipsRedundantMembershipWrites: a no-op export must not rewrite
// the community records. The refresh has to consider every community, so
// without the digest comparison it would commit one write per community on
// every run, forever.
func TestExportSkipsRedundantMembershipWrites(t *testing.T) {
	s, cfg := partitionFixture(t, 6)
	opts := OkfwOptions{Bundle: cfg, CommunitiesOnly: true}
	partition(t, s, map[string][]int{"alpha": {0, 1, 2}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	before, err := s.GetExport(communityKey("alpha"))
	if err != nil || before == nil {
		t.Fatal("record missing")
	}
	if len(before.MemberDigest) != 3*8 {
		t.Fatalf("digest = %d bytes, want 3 packed hashes", len(before.MemberDigest))
	}

	// Unchanged membership → nothing to write.
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	after, err := s.GetExport(communityKey("alpha"))
	if err != nil || after == nil {
		t.Fatal("record missing after re-export")
	}
	if !bytes.Equal(before.MemberDigest, after.MemberDigest) {
		t.Error("digest changed across a no-op export")
	}

	// Changed membership → the record must follow.
	partition(t, s, map[string][]int{"alpha": {0, 1, 2, 3, 4}})
	if _, err := ToOkfw("/repo/myproj", s, opts); err != nil {
		t.Fatal(err)
	}
	grown, err := s.GetExport(communityKey("alpha"))
	if err != nil || grown == nil {
		t.Fatal("record missing after growth")
	}
	if len(grown.MemberDigest) != 5*8 {
		t.Errorf("digest = %d bytes, want 5 packed hashes", len(grown.MemberDigest))
	}
}

// TestExportKeepStalePreservesVanishedConcepts covers OkfwOptions.KeepStale,
// which had no test at all — the escape hatch for the stale sweep was
// unverified while the sweep itself was covered.
func TestExportKeepStalePreservesVanishedConcepts(t *testing.T) {
	s, cfg := fixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	before := len(conceptFiles(t, cfg, res.Dir))

	// Drop every node; without KeepStale the sweep would delete the concepts.
	if err := s.PurgeFile("all.go"); err != nil {
		t.Fatal(err)
	}
	res, err = ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3, KeepStale: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 0 {
		t.Errorf("KeepStale must not delete anything, got Deleted = %d", res.Deleted)
	}
	if after := len(conceptFiles(t, cfg, res.Dir)); after != before {
		t.Errorf("KeepStale changed the concept count from %d to %d", before, after)
	}

	// Without it, the same export sweeps them.
	res, err = ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted == 0 {
		t.Error("without KeepStale the stale sweep should have deleted concepts")
	}
}

// TestExportNoRationaleSkipsRationaleConcepts covers OkfwOptions.NoRationale.
func TestExportNoRationaleSkipsRationaleConcepts(t *testing.T) {
	s, cfg := rationaleFixture(t)

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRationaleConcept(t, cfg, res.Dir) {
		t.Fatal("fixture should export a rationale concept by default")
	}

	s2, cfg2 := rationaleFixture(t)
	res, err = ToOkfw("/repo/myproj", s2, OkfwOptions{Bundle: cfg2, MinDegree: 3, NoRationale: true})
	if err != nil {
		t.Fatal(err)
	}
	if hasRationaleConcept(t, cfg2, res.Dir) {
		t.Error("NoRationale must skip rationale concepts entirely")
	}
}

// TestExportGeneratorReachesFrontmatter covers OkfwOptions.Generator, which was
// only exercised by the Cypher writer.
func TestExportGeneratorReachesFrontmatter(t *testing.T) {
	s, cfg := fixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{
		Bundle: cfg, MinDegree: 3, Generator: "graphy 9.9.9-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for name, content := range conceptFiles(t, cfg, res.Dir) {
		if strings.Contains(string(content), "graphy 9.9.9-test") {
			found = true
		} else if strings.Contains(string(content), "node_id:") {
			t.Errorf("%s has a graphy block but no generator stamp", name)
		}
	}
	if !found {
		t.Error("Generator never reached the frontmatter")
	}
}

// TestExportCommunitiesOnlyRemovesNodeConcepts documents a footgun rather than
// a bug: narrowing what is exported makes the previously-exported node
// concepts stale, and the sweep then deletes them. --keep-stale is the escape
// hatch. Pinning it here means a future change to the sweep cannot silently
// turn a narrowing flag into a destructive one, or vice versa.
func TestExportCommunitiesOnlyRemovesNodeConcepts(t *testing.T) {
	s, cfg := fixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(conceptFiles(t, cfg, res.Dir)) != 4 {
		t.Fatalf("expected 4 concepts before narrowing, got %d", len(conceptFiles(t, cfg, res.Dir)))
	}

	res, err = ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3, CommunitiesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 2 {
		t.Errorf("CommunitiesOnly should sweep the 2 node concepts, got Deleted = %d", res.Deleted)
	}
	if n := len(conceptFiles(t, cfg, res.Dir)); n != 2 { // overview + community
		t.Errorf("concepts remaining = %d, want 1 (the community)", n)
	}
}

// hasRationaleConcept matches on the frontmatter node id, not on body text:
// a rationale's prose also appears in its owner's edge list, so a substring
// search would find the wrong file.
func hasRationaleConcept(t *testing.T, cfg *config.Config, dir string) bool {
	t.Helper()
	name, _, _ := findByNodeID(t, conceptFiles(t, cfg, dir), whyNodeID)
	return name != ""
}

// TestExportRefusesAChangedProjectDirectory: --project selects the output
// directory, but live export records keep pointing into the old one. linkTo
// emits a bare "./<file>.md" for every link, so a split across two directories
// silently resolves every cross-directory reference to nothing.
func TestExportRefusesAChangedProjectDirectory(t *testing.T) {
	s, cfg := fixture(t)
	if _, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3}); err != nil {
		t.Fatal(err)
	}
	_, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3, Project: "renamed"})
	if err == nil {
		t.Fatal("want an error when the export directory moves out from under live concepts")
	}
	if !strings.Contains(err.Error(), "graphy/myproj") || !strings.Contains(err.Error(), "graphy/renamed") {
		t.Errorf("error should name both directories, got: %v", err)
	}
}

// TestFilenameEncodes covers the guard that keeps an in-place re-key in place.
func TestFilenameEncodes(t *testing.T) {
	const id = "019fa49e-b948-7000-8000-000000000001"
	cases := []struct {
		name, title string
		want        bool
	}{
		{"community-core--019fa49eb948.md", "Community: core", true},
		{"community-core--019fa49eb9487000.md", "Community: core", true}, // a longer shortid still encodes it
		{"community-core--019fa49eb948.md", "Community: other", false},
		{"community-other--019fa49eb948.md", "Community: core", false},
		{"community-core--deadbeef1234.md", "Community: core", false}, // different concept
		{"community-core.md", "Community: core", false},
		{"community-core--019fa49eb948.txt", "Community: core", false},
	}
	for _, c := range cases {
		if got := filenameEncodes(c.name, c.title, id); got != c.want {
			t.Errorf("filenameEncodes(%q, %q) = %v, want %v", c.name, c.title, got, c.want)
		}
	}
}

// docFixture builds a store holding a markdown document whose section
// references a code entity, plus the file and heading nodes that never become
// concepts of their own.
func docFixture(t *testing.T) (*store.Store, *config.Config) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	nodes := []model.Node{
		{ID: "go:file:a.go#", Label: "a.go", Kind: model.KindFile, Language: "go", File: "a.go",
			Span: model.Span{StartLine: 1}, Degree: 4},
		{ID: "go:function:a.go#Alpha", Label: "Alpha", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 3}, Degree: 4},
		{ID: "markdown:doc:docs/guide.md#", Label: "docs/guide.md", Kind: model.KindDoc,
			Language: "markdown", File: "docs/guide.md", Span: model.Span{StartLine: 1},
			Doc: "Guide", Degree: 4},
		{ID: "markdown:heading:docs/guide.md#usage", Label: "Usage", Kind: model.KindHeading,
			Language: "markdown", File: "docs/guide.md", Span: model.Span{StartLine: 5}, Degree: 2},
	}
	if err := s.PutFile(store.FileRecord{Path: "all", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		// Structural noise: a file containing its function, a doc containing
		// its heading. Neither endpoint is ever a concept.
		{Source: "go:file:a.go#", Target: "go:function:a.go#Alpha",
			Relation: model.RelContains, Confidence: model.ConfExtracted},
		{Source: "markdown:doc:docs/guide.md#", Target: "markdown:heading:docs/guide.md#usage",
			Relation: model.RelContains, Confidence: model.ConfExtracted},
		// The link that matters: a documentation section describing code.
		{Source: "markdown:heading:docs/guide.md#usage", Target: "go:function:a.go#Alpha",
			Relation: model.RelReferences, Confidence: model.ConfInferred},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCommunities(
		map[string]int{"go:function:a.go#Alpha": 0, "markdown:doc:docs/guide.md#": 0},
		map[int]string{0: "core"}); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(t.TempDir(), ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	return s, config.ConfigFor(bundleRoot)
}

// bodyOfTitle returns the rendered concept whose frontmatter title matches.
func bodyOfTitle(t *testing.T, cfg *config.Config, dir, title string) string {
	t.Helper()
	for name, content := range conceptFiles(t, cfg, dir) {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatalf("%s fails validation: %v", name, err)
		}
		if z.Title == title {
			return string(content)
		}
	}
	t.Fatalf("no concept titled %q in %s", title, dir)
	return ""
}

// TestExportDropsStructuralDeadEnds: file and heading nodes never become
// concepts, so `contains` edges naming them rendered as unlinked text that
// restated resource: and the outline. On this repository they were a fifth of
// every bullet the export produced.
func TestExportDropsStructuralDeadEnds(t *testing.T) {
	s, cfg := docFixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	alpha := bodyOfTitle(t, cfg, res.Dir, "Alpha")
	if strings.Contains(alpha, "contains") {
		t.Errorf("Alpha still lists a contains edge to its file:\n%s", alpha)
	}
	// The reference from a documentation section survives, redirected to the
	// document concept — the heading itself is not a concept to link to.
	if !strings.Contains(alpha, "### references (1)") {
		t.Errorf("Alpha lost its incoming doc reference:\n%s", alpha)
	}
	if !strings.Contains(alpha, "[Usage](./docs-guide-md--") {
		t.Errorf("doc reference should link to the containing document:\n%s", alpha)
	}
}

// TestExportRendersDocOutline: headings hold a document's references but are
// never exported, so without an outline a doc concept showed a flat list of
// unlinked heading names and no sign of what any section was about.
func TestExportRendersDocOutline(t *testing.T) {
	s, cfg := docFixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	doc := bodyOfTitle(t, cfg, res.Dir, "docs/guide.md")
	if !strings.Contains(doc, "## Outline") {
		t.Errorf("doc concept has no outline:\n%s", doc)
	}
	if !strings.Contains(doc, "- Usage — [Alpha](./alpha--") {
		t.Errorf("outline should say which section describes which entity:\n%s", doc)
	}
}

// TestExportOverviewIsTheBundleEntryPoint pins the front door: without it an
// agent arriving at graphy/<project>/ finds a thousand sibling concepts and no
// starting point, and the clustering settings every community concept depends
// on are unrecoverable from the bundle.
func TestExportOverviewIsTheBundleEntryPoint(t *testing.T) {
	s, cfg := fixture(t)
	// A real build records this; the bundle is the only place it survives.
	if err := s.PutClusterMeta(store.ClusterMeta{Algorithm: "leiden", Resolution: 1, Seed: 1}); err != nil {
		t.Fatal(err)
	}
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOfTitle(t, cfg, res.Dir, "Code graph: myproj")
	for _, want := range []string{
		"## Subsystems (1)",
		"[core](./community-core--",
		"## Entry points",
		"3 nodes, 2 edges",
		"Clustering: leiden",
		"--min-degree 3",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("overview missing %q:\n%s", want, body)
		}
	}

	// Communities parent to the overview, so the bundle is a tree rather than
	// a thousand siblings.
	var overviewID, communityParent string
	for _, content := range conceptFiles(t, cfg, res.Dir) {
		z, err := config.ParseAndValidate(content)
		if err != nil {
			t.Fatal(err)
		}
		switch z.Type {
		case "code-graph":
			overviewID = z.ID
		case "code-community":
			communityParent = z.Parent
		}
	}
	if overviewID == "" || communityParent != overviewID {
		t.Errorf("community parent = %q, want the overview id %q", communityParent, overviewID)
	}
}

// TestExportCapsEdgeGroups: a hub's edge list is the difference between a
// concept an agent can afford to read and one it cannot. Truncation has to be
// visible and has to say how to get the rest.
func TestExportCapsEdgeGroups(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	nodes := []model.Node{{ID: "go:function:hub.go#Hub", Label: "Hub", Kind: model.KindFunction,
		Language: "go", File: "hub.go", Span: model.Span{StartLine: 1}, Degree: 12}}
	var edges []model.Edge
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("go:function:c%d.go#Callee%d", i, i)
		nodes = append(nodes, model.Node{ID: id, Label: fmt.Sprintf("Callee%d", i),
			Kind: model.KindFunction, Language: "go", File: fmt.Sprintf("c%d.go", i),
			Span: model.Span{StartLine: 1}, Degree: 3})
		edges = append(edges, model.Edge{Source: "go:function:hub.go#Hub", Target: id,
			Relation: model.RelCalls, Confidence: model.ConfExtracted})
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges(edges); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(t.TempDir(), ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.ConfigFor(bundleRoot)

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3, MaxEdges: 5})
	if err != nil {
		t.Fatal(err)
	}
	hub := bodyOfTitle(t, cfg, res.Dir, "Hub")
	// The heading carries the true count even though the list is cut.
	if !strings.Contains(hub, "### calls (12)") {
		t.Errorf("relation heading should carry the full count:\n%s", hub)
	}
	if n := strings.Count(hub, "- [Callee"); n != 5 {
		t.Errorf("listed %d callees, want 5 (the cap)", n)
	}
	if !strings.Contains(hub, "… 7 more — `graphy neighbors <entity> --rel calls`") {
		t.Errorf("truncation must be reported with the query that returns the rest:\n%s", hub)
	}
}

// TestExportSegregatesAmbiguousEdges: mixed into the list an unresolved edge
// reads exactly like a resolved one while being the least trustworthy thing on
// the page.
func TestExportSegregatesAmbiguousEdges(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	nodes := []model.Node{
		{ID: "go:function:a.go#Caller", Label: "Caller", Kind: model.KindFunction, Language: "go",
			File: "a.go", Span: model.Span{StartLine: 1}, Degree: 3},
		{ID: "go:function:b.go#Sure", Label: "Sure", Kind: model.KindFunction, Language: "go",
			File: "b.go", Span: model.Span{StartLine: 1}, Degree: 3},
		{ID: "go:function:c.go#Maybe", Label: "Maybe", Kind: model.KindFunction, Language: "go",
			File: "c.go", Span: model.Span{StartLine: 1}, Degree: 3},
	}
	if err := s.PutFile(store.FileRecord{Path: "all.go", NodeIDs: ids(nodes)}, nodes); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEdges([]model.Edge{
		{Source: nodes[0].ID, Target: nodes[1].ID, Relation: model.RelCalls, Confidence: model.ConfExtracted},
		{Source: nodes[0].ID, Target: nodes[2].ID, Relation: model.RelCalls, Confidence: model.ConfAmbiguous},
	}); err != nil {
		t.Fatal(err)
	}
	bundleRoot := filepath.Join(t.TempDir(), ".okfw")
	if err := os.MkdirAll(bundleRoot, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := config.ConfigFor(bundleRoot)

	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}
	body := bodyOfTitle(t, cfg, res.Dir, "Caller")
	if !strings.Contains(body, "- [Sure](./sure--") {
		t.Errorf("resolved edge should be an ordinary bullet:\n%s", body)
	}
	if !strings.Contains(body, "- Ambiguous (unresolved between several targets): [Maybe](./maybe--") {
		t.Errorf("ambiguous edge should be segregated:\n%s", body)
	}
}

// TestExportRecordsTheBundleAndBridgesToConcepts pins the node→concept mapping
// that lets the two tools name the same entity.
//
// The mapping was always persisted and never readable: an export record holds a
// *bundle-relative* path and nothing identified the bundle, so even having found
// the record there was no path to open.
func TestExportRecordsTheBundleAndBridgesToConcepts(t *testing.T) {
	s, cfg := fixture(t)
	res, err := ToOkfw("/repo/myproj", s, OkfwOptions{Bundle: cfg, MinDegree: 3})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetMeta(store.MetaBundleKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg.RootPath {
		t.Errorf("recorded bundle = %q, want %q", got, cfg.RootPath)
	}

	c, err := query.ConceptFor(s, "go:function:a.go#Alpha")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("Alpha has no concept mapping")
	}
	if !strings.HasPrefix(c.RelPath, res.Dir+"/alpha--") {
		t.Errorf("concept path = %q, want one under %s", c.RelPath, res.Dir)
	}
	// The absolute path has to be openable, not merely well-formed.
	if _, err := os.Stat(c.Path); err != nil {
		t.Errorf("concept abs path is not readable: %v", err)
	}
	// And the record's concept id must match what the file actually carries.
	content, err := os.ReadFile(c.Path)
	if err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatal(err)
	}
	if z.ID != c.ConceptID {
		t.Errorf("mapping says %q, file says %q", c.ConceptID, z.ID)
	}
	if z.Graphy == nil || z.Graphy.NodeID != c.NodeID {
		t.Errorf("concept does not carry its node id back: %+v", z.Graphy)
	}

	// An entity below --min-degree was never exported, which is a nil mapping
	// rather than an error.
	if c, err := query.ConceptFor(s, "go:function:c.go#Tiny"); err != nil || c != nil {
		t.Errorf("unexported entity should map to nothing, got %+v (%v)", c, err)
	}

	idx, err := query.ConceptIndex(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx["go:function:a.go#Alpha"]; !ok {
		t.Errorf("ConceptIndex missing Alpha: %v", idx)
	}
}
