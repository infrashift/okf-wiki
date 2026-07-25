// Package build orchestrates the graphy pipeline: incremental extraction
// (hash-diffed against the store's file cache) followed by a global two-pass
// symbol resolution that turns unresolved refs into confidence-tagged edges.
//
// Extraction is incremental; resolution always runs globally — it is an
// in-memory pass over cached records and stays cheap even at 100k symbols.
package build

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/analyze"
	"okfw/cli/internal/graphy/cluster"
	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/extract"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/report"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/graphy/workspace"
)

// syntheticPath is the pseudo file record that owns nodes belonging to no
// source file (external modules, internal package nodes). It is rewritten on
// every build; detect never lists .graphy/, so it cannot collide.
const syntheticPath = ".graphy/synthetic"

// ambiguousCap is the most candidate targets an unresolved reference may fan
// out to before it is dropped instead of tagged AMBIGUOUS.
const ambiguousCap = 5

// Options configures a build run.
type Options struct {
	Langs   []string // subset of languages ("" = all supported)
	Full    bool     // wipe the extraction cache first
	Cluster ClusterOptions
}

// ClusterOptions selects the community detector. The zero value means Leiden
// at resolution 1 with seed 1 — which is deliberate: several callers (the MCP
// update tool, `graphy update`) construct a bare Options, and they must get
// the same defaults as an explicit `graphy build`.
type ClusterOptions struct {
	Algorithm  cluster.Algorithm // "" → cluster.DefaultAlgorithm
	Resolution float64           // 0 → cluster.DefaultResolution
	Seed       uint64            // 0 → cluster.DefaultSeed
}

// normalized fills in defaults and rejects invalid values. Validation lives
// here rather than only at the CLI so that every caller is covered.
func (c ClusterOptions) normalized() (ClusterOptions, error) {
	algo, err := cluster.ParseAlgorithm(string(c.Algorithm))
	if err != nil {
		return c, err
	}
	c.Algorithm = algo
	if c.Resolution < 0 || math.IsNaN(c.Resolution) || math.IsInf(c.Resolution, 0) {
		return c, fmt.Errorf("invalid clustering resolution %v (want a finite value >= 0)", c.Resolution)
	}
	if c.Resolution == 0 {
		c.Resolution = cluster.DefaultResolution
	}
	if c.Seed == 0 {
		c.Seed = cluster.DefaultSeed
	}
	return c, nil
}

// Stats summarizes what a build did.
type Stats struct {
	FilesScanned   int
	FilesExtracted int
	FilesPurged    int
	Nodes          int
	Edges          int
	Communities    int
	RefsResolved   int
	RefsAmbiguous  int
	RefsDropped    int
	// ClusterAlgorithm is the detector that produced Communities.
	ClusterAlgorithm string
}

// Run executes the pipeline against the workspace's store.
func Run(w *workspace.Workspace, s *store.Store, opts Options) (*Stats, error) {
	// Validate before any extraction work, so a bad flag fails fast.
	clusterOpts, err := opts.Cluster.normalized()
	if err != nil {
		return nil, err
	}
	opts.Cluster = clusterOpts

	session, err := extract.NewSession(opts.Langs)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	files, err := detect.List(w.Root)
	if err != nil {
		return nil, err
	}
	var supported []detect.File
	for _, f := range files {
		if session.Supported(f.Language) {
			supported = append(supported, f)
		}
	}

	stats := &Stats{FilesScanned: len(supported)}

	if opts.Full {
		var stale []string
		if err := s.EachFile(func(rec store.FileRecord) error {
			stale = append(stale, rec.Path)
			return nil
		}); err != nil {
			return nil, err
		}
		for _, p := range stale {
			if err := s.PurgeFile(p); err != nil {
				return nil, err
			}
		}
	}

	// Diff against the cache: extract new/changed, purge deleted.
	live := map[string]bool{}
	for _, f := range supported {
		live[f.Path] = true
		cached, err := s.GetFile(f.Path)
		if err != nil {
			return nil, err
		}
		if cached != nil && cached.SHA256 == f.SHA256 {
			continue
		}
		if err := extractFile(session, s, f); err != nil {
			return nil, fmt.Errorf("extracting %s: %w", f.Path, err)
		}
		stats.FilesExtracted++
	}
	var deleted []string
	if err := s.EachFile(func(rec store.FileRecord) error {
		if rec.Path != syntheticPath && !live[rec.Path] {
			deleted = append(deleted, rec.Path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	for _, p := range deleted {
		if err := s.PurgeFile(p); err != nil {
			return nil, err
		}
		stats.FilesPurged++
	}

	if err := resolve(w, s, stats); err != nil {
		return nil, err
	}
	if err := s.RecomputeDegrees(); err != nil {
		return nil, err
	}
	if err := clusterAndReport(w, s, opts.Cluster, stats); err != nil {
		return nil, err
	}

	stats.Nodes, stats.Edges, err = s.Counts()
	return stats, err
}

// clusterAndReport detects communities with the configured deterministic
// detector, labels them, and renders GRAPH_REPORT.md.
func clusterAndReport(w *workspace.Workspace, s *store.Store, opts ClusterOptions, stats *Stats) error {
	var nodes []model.Node
	var nodeIDs []string
	if err := s.EachNode(func(n model.Node) error {
		nodes = append(nodes, n)
		nodeIDs = append(nodeIDs, n.ID)
		return nil
	}); err != nil {
		return err
	}
	var pairs [][2]string
	if err := s.EachEdge(func(e model.Edge) error {
		pairs = append(pairs, [2]string{e.Source, e.Target})
		return nil
	}); err != nil {
		return err
	}

	detector, err := cluster.New(opts.Algorithm, opts.Resolution, opts.Seed)
	if err != nil {
		return err
	}
	assign, err := detector.Detect(nodeIDs, pairs)
	if err != nil {
		return err
	}
	if err := s.SetCommunities(assign, analyze.CommunityLabels(nodes, assign)); err != nil {
		return err
	}
	if err := s.PutClusterMeta(store.ClusterMeta{
		Algorithm:  string(opts.Algorithm),
		Resolution: opts.Resolution,
		Seed:       opts.Seed,
	}); err != nil {
		return err
	}
	// Drop the key this replaced; it was never read back, which is exactly how
	// it rotted into recording an algorithm that was no longer the default.
	if err := s.DeleteMeta("louvain_seed"); err != nil {
		return err
	}
	stats.ClusterAlgorithm = string(opts.Algorithm)

	rep, err := analyze.Build(s, w.Root)
	if err != nil {
		return err
	}
	stats.Communities = rep.Stats.Communities
	f, err := os.Create(w.ReportPath())
	if err != nil {
		return err
	}
	defer f.Close()
	return report.Render(f, rep)
}

func extractFile(session *extract.Session, s *store.Store, f detect.File) error {
	src, err := os.ReadFile(f.AbsPath)
	if err != nil {
		return err
	}
	res, err := session.Extract(f, src)
	if err != nil {
		return err
	}
	ids := make([]string, len(res.Nodes))
	for i := range res.Nodes {
		ids[i] = res.Nodes[i].ID
	}
	rec := store.FileRecord{
		Path:    f.Path,
		SHA256:  f.SHA256,
		Size:    f.Size,
		NodeIDs: ids,
		Refs:    res.Refs,
		Edges:   res.Edges,
	}
	return s.PutFile(rec, res.Nodes)
}

// resolve turns every cached ref into edges (pass 2), synthesizes external
// and internal-package nodes, and rewrites the edge buckets wholesale.
func resolve(w *workspace.Workspace, s *store.Store, stats *Stats) error {
	// Load resolution inputs into memory.
	nodes := map[string]model.Node{}
	symbols := map[string][]string{} // lang \x00 name → node ids
	if err := s.EachNode(func(n model.Node) error {
		nodes[n.ID] = n
		return nil
	}); err != nil {
		return err
	}
	for _, n := range nodes {
		for _, key := range symbolMemKeys(n) {
			symbols[key] = append(symbols[key], n.ID)
		}
	}
	var recs []store.FileRecord
	fileFirstNode := map[string]string{} // rel path → file-level node id
	if err := s.EachFile(func(rec store.FileRecord) error {
		if rec.Path == syntheticPath {
			return nil
		}
		recs = append(recs, rec)
		if len(rec.NodeIDs) > 0 {
			fileFirstNode[rec.Path] = rec.NodeIDs[0]
		}
		return nil
	}); err != nil {
		return err
	}
	goModule := goModuleName(w.Root)
	dirHasFiles := map[string]bool{}
	for path := range fileFirstNode {
		if i := strings.LastIndex(path, "/"); i >= 0 {
			dirHasFiles[path[:i]] = true
		} else {
			dirHasFiles["."] = true
		}
	}

	edges := map[string]model.Edge{}
	synthetic := map[string]model.Node{}
	addEdge := func(e model.Edge) {
		key := e.Source + "\x00" + string(e.Relation) + "\x00" + e.Target
		if prev, ok := edges[key]; ok && confRank(prev.Confidence) >= confRank(e.Confidence) {
			return
		}
		edges[key] = e
	}

	for _, rec := range recs {
		for _, e := range rec.Edges {
			addEdge(e)
		}
		lang := detect.Language(rec.Path)
		for _, ref := range rec.Refs {
			switch ref.Relation {
			case model.RelReferences:
				// Path-shaped ref (markdown links, config values): target any
				// extracted file. Otherwise fall back to the symbol index —
				// terraform addresses (var.x, aws_instance.web) and CUE
				// definitions (#Concept) resolve like code symbols.
				if target, ok := fileFirstNode[ref.Name]; ok {
					addEdge(model.Edge{
						Source: ref.FromID, Target: target,
						Relation: model.RelReferences, Confidence: model.ConfExtracted,
						File: rec.Path, Line: ref.Line,
					})
					stats.RefsResolved++
				} else {
					candidates := lookupSymbol(symbols, lang, ref.Name)
					resolveCandidates(ref, rec.Path, candidates, nodes, addEdge, stats)
				}

			case model.RelImports:
				target := resolveImport(lang, goModule, ref.Name, rec.Path, dirHasFiles, fileFirstNode, synthetic)
				addEdge(model.Edge{
					Source: ref.FromID, Target: target,
					Relation: model.RelImports, Confidence: model.ConfInferred,
					File: rec.Path, Line: ref.Line,
				})
				stats.RefsResolved++

			case model.RelDefines:
				// Reversed: the resolved type owns ref.FromID (the method).
				if owner, conf := resolveUnique(symbols, nodes, lang, ref.Name, rec.Path); owner != "" {
					addEdge(model.Edge{
						Source: owner, Target: ref.FromID,
						Relation: model.RelDefines, Confidence: conf,
						File: rec.Path, Line: ref.Line,
					})
					stats.RefsResolved++
				} else {
					stats.RefsDropped++
				}

			default: // calls and other symbol-shaped refs
				candidates := lookupSymbol(symbols, lang, ref.Name)
				resolveCandidates(ref, rec.Path, candidates, nodes, addEdge, stats)
			}
		}
	}

	// Persist synthetic nodes under their pseudo file record.
	ids := make([]string, 0, len(synthetic))
	nodeList := make([]model.Node, 0, len(synthetic))
	for _, n := range synthetic {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		nodeList = append(nodeList, synthetic[id])
	}
	if err := s.PutFile(store.FileRecord{Path: syntheticPath, NodeIDs: ids}, nodeList); err != nil {
		return err
	}

	all := make([]model.Edge, 0, len(edges))
	for _, e := range edges {
		all = append(all, e)
	}
	return s.ReplaceEdges(all)
}

// resolveCandidates applies the confidence ladder to a symbol-shaped ref:
// same-file → EXTRACTED, unique global → INFERRED, 2..cap → AMBIGUOUS, else drop.
func resolveCandidates(ref model.Ref, siteFile string, candidates []string, nodes map[string]model.Node, addEdge func(model.Edge), stats *Stats) {
	if len(candidates) == 0 {
		stats.RefsDropped++
		return
	}
	var sameFile []string
	for _, id := range candidates {
		if nodes[id].File == siteFile {
			sameFile = append(sameFile, id)
		}
	}
	emit := func(targets []string, conf model.Confidence) {
		for _, id := range targets {
			if id == ref.FromID {
				continue // a def never resolves to itself via its own name
			}
			addEdge(model.Edge{
				Source: ref.FromID, Target: id,
				Relation: ref.Relation, Confidence: conf,
				File: siteFile, Line: ref.Line,
			})
		}
	}
	switch {
	case len(sameFile) == 1:
		emit(sameFile, model.ConfExtracted)
		stats.RefsResolved++
	case len(candidates) == 1:
		emit(candidates, model.ConfInferred)
		stats.RefsResolved++
	case len(candidates) <= ambiguousCap:
		emit(candidates, model.ConfAmbiguous)
		stats.RefsAmbiguous++
	default:
		stats.RefsDropped++
	}
}

// resolveUnique returns a single owner id for a name: same-file first, then a
// unique global match; "" when unresolvable.
func resolveUnique(symbols map[string][]string, nodes map[string]model.Node, lang, name, siteFile string) (string, model.Confidence) {
	candidates := lookupSymbol(symbols, lang, name)
	var sameFile []string
	for _, id := range candidates {
		if nodes[id].File == siteFile {
			sameFile = append(sameFile, id)
		}
	}
	if len(sameFile) == 1 {
		return sameFile[0], model.ConfExtracted
	}
	if len(candidates) == 1 {
		return candidates[0], model.ConfInferred
	}
	return "", model.ConfInferred
}

// lookupSymbol tries the full (possibly qualified) name, then its bare tail.
func lookupSymbol(symbols map[string][]string, lang, name string) []string {
	if ids := symbols[lang+"\x00"+name]; len(ids) > 0 {
		return ids
	}
	if i := strings.LastIndex(name, "."); i >= 0 && i+1 < len(name) {
		return symbols[lang+"\x00"+name[i+1:]]
	}
	return nil
}

// resolveImport returns (creating on demand) the node an import edge targets:
// an internal package/file node when the import maps into the repo, else an
// external module node.
func resolveImport(lang, goModule, importPath, sitePath string, dirHasFiles map[string]bool, fileFirstNode map[string]string, synthetic map[string]model.Node) string {
	switch lang {
	case "go":
		if goModule != "" {
			if rel, ok := strings.CutPrefix(importPath, goModule); ok {
				rel = strings.TrimPrefix(rel, "/")
				if rel == "" {
					rel = "."
				}
				if dirHasFiles[rel] {
					id := model.NodeID("go", model.KindModule, rel, rel)
					if _, ok := synthetic[id]; !ok {
						synthetic[id] = model.Node{
							ID: id, Label: rel, Kind: model.KindModule, Language: "go",
						}
					}
					return id
				}
			}
		}
	case "javascript", "typescript":
		if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
			base := joinRel(dirOf(sitePath), importPath)
			for _, cand := range scriptImportCandidates(base) {
				if id, ok := fileFirstNode[cand]; ok {
					return id
				}
			}
		}
	case "python":
		base := strings.ReplaceAll(importPath, ".", "/")
		for _, cand := range []string{base + ".py", base + "/__init__.py"} {
			if id, ok := fileFirstNode[cand]; ok {
				return id
			}
		}
	case "yaml":
		// Ansible role references ("role:<name>") resolve by directory
		// convention: the role's entry file if extracted, else a module node
		// for the role directory, else external.
		if name, ok := strings.CutPrefix(importPath, "role:"); ok {
			bases := []string{"roles/" + name}
			if d := dirOf(sitePath); d != "" {
				bases = append([]string{joinRel(d, "roles/"+name)}, bases...)
			}
			for _, base := range bases {
				for _, entry := range []string{"/tasks/main.yml", "/tasks/main.yaml", "/meta/main.yml", "/defaults/main.yml"} {
					if id, ok := fileFirstNode[base+entry]; ok {
						return id
					}
				}
			}
			for _, base := range bases {
				for dir := range dirHasFiles {
					if dir == base || strings.HasPrefix(dir, base+"/") {
						id := model.NodeID("yaml", model.KindModule, base, base)
						if _, ok := synthetic[id]; !ok {
							synthetic[id] = model.Node{
								ID: id, Label: "role:" + name, Kind: model.KindModule, Language: "yaml",
							}
						}
						return id
					}
				}
			}
		}
	case "terraform":
		// Local module sources ("./modules/vpc") become internal module
		// nodes when the directory holds extracted files; registry sources
		// fall through to external nodes.
		if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
			dir := joinRel(dirOf(sitePath), importPath)
			if dirHasFiles[dir] {
				id := model.NodeID("terraform", model.KindModule, dir, dir)
				if _, ok := synthetic[id]; !ok {
					synthetic[id] = model.Node{
						ID: id, Label: dir, Kind: model.KindModule, Language: "terraform",
					}
				}
				return id
			}
		}
	}
	id := model.NodeID(lang, model.KindExternal, "", importPath)
	if _, ok := synthetic[id]; !ok {
		synthetic[id] = model.Node{
			ID: id, Label: importPath, Kind: model.KindExternal, Language: lang, External: true,
		}
	}
	return id
}

// scriptImportCandidates lists the repo paths a JS/TS relative import may
// resolve to, in preference order.
func scriptImportCandidates(base string) []string {
	out := []string{base}
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"} {
		out = append(out, base+ext)
	}
	for _, idx := range []string{"/index.ts", "/index.tsx", "/index.js", "/index.jsx"} {
		out = append(out, base+idx)
	}
	return out
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return ""
}

// joinRel resolves a ./ or ../ href against a slash-separated directory.
func joinRel(dir, href string) string {
	parts := []string{}
	if dir != "" {
		parts = strings.Split(dir, "/")
	}
	for _, seg := range strings.Split(href, "/") {
		switch seg {
		case "", ".":
		case "..":
			if len(parts) > 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, seg)
		}
	}
	return strings.Join(parts, "/")
}

// symbolMemKeys mirrors the store's symbol indexing for the in-memory pass.
func symbolMemKeys(n model.Node) []string {
	switch n.Kind {
	case model.KindFile, model.KindDoc, model.KindHeading, model.KindConfig, model.KindExternal:
		return nil
	}
	if n.External || n.Label == "" {
		return nil
	}
	keys := []string{n.Language + "\x00" + n.Label}
	if i := strings.LastIndex(n.Label, "."); i >= 0 && i+1 < len(n.Label) {
		keys = append(keys, n.Language+"\x00"+n.Label[i+1:])
	}
	return keys
}

func confRank(c model.Confidence) int {
	switch c {
	case model.ConfExtracted:
		return 3
	case model.ConfInferred:
		return 2
	default:
		return 1
	}
}

var goModuleRe = regexp.MustCompile(`(?m)^module\s+(\S+)`)

// goModuleName reads the module path from root/go.mod ("" when absent).
func goModuleName(root string) string {
	content, err := os.ReadFile(root + "/go.mod")
	if err != nil {
		return ""
	}
	if m := goModuleRe.FindSubmatch(content); m != nil {
		return string(m[1])
	}
	return ""
}
