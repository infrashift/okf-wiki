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
	if second.Recovered != 1 {
		t.Errorf("Recovered = %d, want 1 (the re-keyed community)", second.Recovered)
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
	if got, err := s.GetExport(newKey); err != nil || got == nil {
		t.Errorf("new key not written: %v", err)
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
	if second.Recovered != 2 {
		t.Errorf("Recovered = %d, want 2 (both same-titled communities)", second.Recovered)
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
	if second.Recovered != 1 {
		t.Errorf("Recovered = %d, want 1 (adopted via membership)", second.Recovered)
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
	if second.Recovered != 0 {
		t.Errorf("Recovered = %d: a disjoint community must not adopt", second.Recovered)
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
	if second.Recovered != 0 {
		t.Errorf("Recovered = %d: an even split has no majority successor", second.Recovered)
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
	if second.Recovered != 1 {
		t.Errorf("Recovered = %d, want exactly 1", second.Recovered)
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
	if third.Recovered != 1 || third.Created != 0 || third.Deleted != 0 {
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
	if third.Recovered != 1 {
		t.Errorf("Recovered = %d, want 1 (adopted via the backfilled digest)", third.Recovered)
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
