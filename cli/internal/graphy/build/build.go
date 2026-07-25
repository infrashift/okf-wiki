// Package build orchestrates the graphy pipeline: incremental extraction
// (hash-diffed against the store's file cache) followed by a global two-pass
// symbol resolution that turns unresolved refs into confidence-tagged edges.
//
// Extraction is incremental; resolution always runs globally — it is an
// in-memory pass over cached records and stays cheap even at 100k symbols.
package build

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/extract"
	"okfw/cli/internal/graphy/model"
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
	Langs []string // subset of languages ("" = all supported)
	Full  bool     // wipe the extraction cache first
}

// Stats summarizes what a build did.
type Stats struct {
	FilesScanned   int
	FilesExtracted int
	FilesPurged    int
	Nodes          int
	Edges          int
	RefsResolved   int
	RefsAmbiguous  int
	RefsDropped    int
}

// Run executes the pipeline against the workspace's store.
func Run(w *workspace.Workspace, s *store.Store, opts Options) (*Stats, error) {
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

	stats.Nodes, stats.Edges, err = s.Counts()
	return stats, err
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
				// Path-shaped ref (markdown links): target any extracted file.
				if target, ok := fileFirstNode[ref.Name]; ok {
					addEdge(model.Edge{
						Source: ref.FromID, Target: target,
						Relation: model.RelReferences, Confidence: model.ConfExtracted,
						File: rec.Path, Line: ref.Line,
					})
					stats.RefsResolved++
				} else {
					stats.RefsDropped++
				}

			case model.RelImports:
				target := resolveImport(lang, goModule, ref.Name, dirHasFiles, synthetic)
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
// an internal package node when the import maps to a directory with extracted
// files, else an external module node.
func resolveImport(lang, goModule, importPath string, dirHasFiles map[string]bool, synthetic map[string]model.Node) string {
	if lang == "go" && goModule != "" {
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
	id := model.NodeID(lang, model.KindExternal, "", importPath)
	if _, ok := synthetic[id]; !ok {
		synthetic[id] = model.Node{
			ID: id, Label: importPath, Kind: model.KindExternal, Language: lang, External: true,
		}
	}
	return id
}

// symbolMemKeys mirrors the store's symbol indexing for the in-memory pass.
func symbolMemKeys(n model.Node) []string {
	switch n.Kind {
	case model.KindFile, model.KindDoc, model.KindHeading, model.KindExternal:
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
