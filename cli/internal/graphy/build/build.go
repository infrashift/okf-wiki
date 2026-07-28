// Package build orchestrates the graphy pipeline: incremental extraction
// (hash-diffed against the store's file cache) followed by a global two-pass
// symbol resolution that turns unresolved refs into confidence-tagged edges.
//
// Extraction is incremental; resolution always runs globally — it is an
// in-memory pass over cached records and stays cheap even at 100k symbols.
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
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

// ambiguousCap is the most candidate targets an unresolved reference may fan
// out to before it is dropped instead of tagged AMBIGUOUS.
const ambiguousCap = 5

// Options configures a build run.
type Options struct {
	Langs     []string // subset of languages ("" = all supported)
	Full      bool     // wipe the extraction cache first
	Cluster   ClusterOptions
	Rationale extract.RationaleOptions
}

// extractorVersion is bumped whenever extraction output changes shape. It
// feeds the fingerprint below, so upgrading graphy re-extracts rather than
// leaving pre-upgrade results in the per-file cache.
const extractorVersion = 6

// metaExtractFingerprintKey holds the fingerprint of the last build's
// extraction configuration.
const metaExtractFingerprintKey = "extract_fingerprint"

// extractFingerprint hashes everything that affects extraction output.
//
// Extraction is cached per file by content sha256, which means new extraction
// capability does NOT invalidate anything: without this, upgrading graphy and
// running `graphy update` would produce a graph containing rationale only for
// the files that happened to change since, silently and indefinitely.
func (o Options) extractFingerprint() string {
	markers := append([]string(nil), o.Rationale.Markers...)
	dirs := append([]string(nil), o.Rationale.ADRDirs...)
	langs := append([]string(nil), o.Langs...)
	sort.Strings(markers)
	sort.Strings(dirs)
	sort.Strings(langs)
	h := sha256.Sum256([]byte(fmt.Sprintf("v%d|rationale=%t|markers=%s|adrdirs=%s|langs=%s",
		extractorVersion, o.Rationale.Enabled,
		strings.Join(markers, ","), strings.Join(dirs, ","), strings.Join(langs, ","))))
	return hex.EncodeToString(h[:])
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
	// FilesPurged counts records dropped because the file left the repo.
	FilesPurged int
	// CacheCleared counts records dropped by --full to force re-extraction.
	// Kept apart from FilesPurged so "purged" keeps meaning "gone from disk".
	CacheCleared  int
	Nodes         int
	Edges         int
	Communities   int
	RefsResolved  int
	RefsAmbiguous int
	RefsDropped   int
	// RefsReceiverScoped counts calls through a receiver qualifier that were
	// recovered into an exact match instead of being dropped as a bare tail.
	// Counted separately because it is the difference between "what calls this
	// entry point" answering and returning nothing.
	RefsReceiverScoped int
	// ClusterAlgorithm is the detector that produced Communities.
	ClusterAlgorithm string
	// Failures lists files that could not be extracted. The build continues
	// past them, so callers must surface these — a silent skip is
	// indistinguishable from a file that legitimately defines nothing.
	Failures []FileError
}

// FileError is one file that failed extraction.
type FileError struct {
	Path string
	Err  error
}

func (e FileError) Error() string { return e.Path + ": " + e.Err.Error() }
func (e FileError) Unwrap() error { return e.Err }

// Run executes the pipeline against the workspace's store.
func Run(w *workspace.Workspace, s *store.Store, opts Options) (*Stats, error) {
	// Validate before any extraction work, so a bad flag fails fast.
	clusterOpts, err := opts.Cluster.normalized()
	if err != nil {
		return nil, err
	}
	opts.Cluster = clusterOpts
	if len(opts.Rationale.Markers) == 0 && len(opts.Rationale.ADRDirs) == 0 && !opts.Rationale.Enabled {
		// A zero-valued Rationale means "unset", not "disabled" — several
		// callers construct a bare Options and must get the same defaults as
		// an explicit `graphy build`. Opting out goes through the CLI flag,
		// which sets Enabled=false with the marker lists still populated.
		opts.Rationale = extract.DefaultRationaleOptions()
	}

	// Force a full re-extract when the extraction configuration changed: the
	// per-file cache keys on content alone and would otherwise hide it.
	fingerprint := opts.extractFingerprint()
	prevFingerprint, err := s.GetMeta(metaExtractFingerprintKey)
	if err != nil {
		return nil, err
	}
	if prevFingerprint != fingerprint {
		opts.Full = true
	}

	session, err := extract.NewSessionWith(opts.Langs, extract.SessionOptions{Rationale: opts.Rationale})
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

	// inScope reports whether a stored file record is this run's business.
	//
	// --langs restricts which files are extracted, not which nodes survive.
	// Without this the language filter also drove the delete diff, so every
	// file of another language looked deleted: `graphy build --langs go` on a
	// mixed repo purged everything non-Go and reported it as routine deletion.
	inScope := func(path string) bool {
		if len(opts.Langs) == 0 {
			return true
		}
		return session.Supported(detect.Language(path))
	}

	stats := &Stats{FilesScanned: len(supported)}

	if opts.Full {
		var stale []string
		if err := s.EachFile(func(rec store.FileRecord) error {
			if rec.Path != store.SyntheticPath && inScope(rec.Path) {
				stale = append(stale, rec.Path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
		for _, p := range stale {
			if err := s.PurgeFile(p); err != nil {
				return nil, err
			}
		}
		// Counted separately from FilesPurged: clearing the cache to rebuild
		// is not the same event as a file disappearing from the repo, and
		// conflating them made the incremental signal meaningless.
		stats.CacheCleared = len(stale)
	}

	// Diff against the cache: extract new/changed, purge deleted.
	//
	// A file that cannot be extracted is recorded and skipped rather than
	// aborting the run. A --full build purges the cache before extracting
	// anything, so aborting midway left the store gutted — nodes deleted,
	// edges from the previous run still pointing at them, and none of the
	// repair passes below reached. One unreadable file taking out a whole
	// repository's graph is the worse failure by far.
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
			stats.Failures = append(stats.Failures, FileError{Path: f.Path, Err: err})
			continue
		}
		stats.FilesExtracted++
	}
	var deleted []string
	if err := s.EachFile(func(rec store.FileRecord) error {
		if rec.Path != store.SyntheticPath && inScope(rec.Path) && !live[rec.Path] {
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

	if err := s.PutMeta(metaExtractFingerprintKey, fingerprint); err != nil {
		return nil, err
	}

	stats.Nodes, stats.Edges, err = s.Counts()
	return stats, err
}

// clusterAndReport detects communities with the configured deterministic
// detector, labels them, and renders GRAPH_REPORT.md.
func clusterAndReport(w *workspace.Workspace, s *store.Store, opts ClusterOptions, stats *Stats) error {
	// Rationale nodes are excluded from detection entirely. They are degree-2
	// leaves that carry no modularity signal, but including them would perturb
	// the partition — and every community id shift rewrites `parent:` on every
	// concept in the exported bundle. Each one inherits its owner's community
	// afterwards instead.
	var nodes []model.Node
	var nodeIDs []string
	rationale := map[string]bool{}
	if err := s.EachNode(func(n model.Node) error {
		nodes = append(nodes, n)
		if n.Kind == model.KindRationale {
			rationale[n.ID] = true
			return nil
		}
		nodeIDs = append(nodeIDs, n.ID)
		return nil
	}); err != nil {
		return err
	}
	var pairs [][2]string
	// owner maps a rationale to the entity it explains, for the inherit pass.
	owner := map[string]string{}
	if err := s.EachEdge(func(e model.Edge) error {
		if rationale[e.Source] {
			if e.Relation == model.RelExplains {
				owner[e.Source] = e.Target
			}
			return nil
		}
		if rationale[e.Target] {
			return nil
		}
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
	// Give each rationale its owner's community, so it sorts and exports
	// alongside the code it explains. An orphan falls back to community 0.
	for id := range rationale {
		assign[id] = assign[owner[id]]
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
	return workspace.WriteFileAtomic(w.ReportPath(), func(f io.Writer) error {
		return report.Render(f, rep)
	})
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
	// Drop the previous run's synthetic nodes before snapshotting, not after.
	// They are rebuilt at the end of this function, but model.Indexable admits
	// KindModule, so a stale module node left in the snapshot can win a
	// resolution — and then PutFile deletes it while ReplaceEdges persists an
	// edge pointing at it.
	if err := s.PurgeFile(store.SyntheticPath); err != nil {
		return err
	}

	// Load resolution inputs into memory.
	nodes := map[string]model.Node{}
	symbols := map[string][]string{} // lang \x00 name → node ids
	if err := s.EachNode(func(n model.Node) error {
		nodes[n.ID] = n
		return nil
	}); err != nil {
		return err
	}
	// anyLang indexes the same symbols by bare name. Decision records name the
	// entities they justify without knowing their implementation language —
	// an ADR is markdown, so a language-keyed lookup would never match — so
	// explains refs fall back to this. Sorted, because the map walk above is
	// unordered and the choice between candidates must not vary run to run.
	anyLang := map[string][]string{}
	for _, n := range nodes {
		for _, key := range symbolMemKeys(n) {
			symbols[key] = append(symbols[key], n.ID)
			if i := strings.IndexByte(key, 0); i >= 0 {
				anyLang[key[i+1:]] = append(anyLang[key[i+1:]], n.ID)
			}
		}
	}
	for _, ids := range anyLang {
		sort.Strings(ids)
	}
	var recs []store.FileRecord
	fileFirstNode := map[string]string{} // rel path → file-level node id
	if err := s.EachFile(func(rec store.FileRecord) error {
		if rec.Path == store.SyntheticPath {
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
	// spanIndex returns a file's declaration node ids, built on demand: only
	// line-anchored links need it, and most repos have none.
	fileNodeIDs := map[string][]string{}
	for _, rec := range recs {
		fileNodeIDs[rec.Path] = rec.NodeIDs
	}
	spanIndex := func(path string) []string { return fileNodeIDs[path] }

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

	structFields := goStructFieldIndex(nodes)
	localTypes := goResolveLocals(nodes, symbols)

	for _, rec := range recs {
		for _, e := range rec.Edges {
			addEdge(e)
		}
		lang := detect.Language(rec.Path)
		var pkgDirs map[string]string
		if lang == "go" {
			pkgDirs = goPackageDirs(rec, goModule, dirHasFiles)
		}
		for _, ref := range rec.Refs {
			switch ref.Relation {
			case model.RelReferences, model.RelExplains:
				// Path-shaped ref (markdown links, config values, ADR links):
				// target any extracted file. Otherwise fall back to the symbol
				// index — terraform addresses (var.x, aws_instance.web), CUE
				// definitions (#Concept) and the entities an ADR names all
				// resolve like code symbols, inheriting the confidence ladder.
				name, anchorLine := splitLineAnchor(ref.Name)
				if target, ok := fileFirstNode[name]; ok {
					// A "#L42" anchor says the link is about one entity, not the
					// file: aim at whatever declaration encloses that line, and
					// fall back to the file node when nothing does.
					if anchorLine > 0 {
						if inner := enclosingNode(spanIndex(name), nodes, anchorLine); inner != "" {
							target = inner
						}
					}
					addEdge(model.Edge{
						Source: ref.FromID, Target: target,
						Relation: ref.Relation, Confidence: model.ConfExtracted,
						File: rec.Path, Line: ref.Line,
					})
					stats.RefsResolved++
				} else {
					candidates, match := lookupSymbol(symbols, lang, name)
					// Prose names entities without knowing their language: an ADR
					// and a design doc are both markdown, so a language-keyed
					// lookup can never hit a Go symbol. Having named the entity
					// deliberately, this is not a blind tail match, so it keeps
					// the full ladder — including AMBIGUOUS when the name is
					// genuinely shared.
					if len(candidates) == 0 && (ref.Relation == model.RelExplains || lang == "markdown") {
						candidates, match = anyLang[name], matchFull
						if len(candidates) == 0 {
							if i := strings.LastIndex(name, "."); i >= 0 && i+1 < len(name) {
								candidates, match = anyLang[name[i+1:]], matchTail
							}
						}
					}
					resolveCandidates(ref, rec.Path, candidates, match, nodes, addEdge, stats)
				}

			case model.RelImports:
				target := resolveImport(lang, goModule, ref.Name, rec.Path, dirHasFiles, fileFirstNode, synthetic)
				addEdge(model.Edge{
					Source: ref.FromID, Target: target,
					Relation: model.RelImports, Confidence: model.ConfInferred,
					File: rec.Path, Line: ref.Line,
				})
				stats.RefsResolved++

			case model.RelTests:
				// A test's subject is production code by definition. Without
				// this, TestFoo in one package resolves onto TestFoo's helper
				// in another test package as readily as onto Foo, and the
				// suite ends up testing itself.
				candidates, match := lookupSymbol(symbols, lang, ref.Name)
				candidates = rejectTests(candidates, nodes)
				if len(candidates) == 0 {
					candidates, match = testPrefixMatch(symbols, nodes, lang, ref.Name)
				}
				resolveCandidates(ref, rec.Path, candidates, match, nodes, addEdge, stats)

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
				candidates, match := lookupSymbol(symbols, lang, ref.Name)
				// A Go call qualified by a package this file imports keeps its
				// qualifier: scoping to that package's directory turns what
				// would be a blind tail match into an exact one.
				if match == matchTail && pkgDirs != nil {
					if dir, ok := pkgDirs[goPackageQualifier(ref.Name)]; ok {
						if scoped := scopeToPackage(candidates, nodes, dir); len(scoped) > 0 {
							candidates, match = scoped, matchFull
						}
					}
				}
				// Likewise for a receiver qualifier: `s.engine.Evaluate` inside a
				// method of Server, where Server declares `engine *engine.Engine`,
				// names Engine.Evaluate exactly. Runs after package scoping so an
				// import qualifier always wins — `engine.New` is the package, not
				// a field.
				if match == matchTail && lang == "go" {
					for _, qual := range goReceiverQualified(ref, nodes, structFields, localTypes) {
						if ids := symbols[lang+"\x00"+qual]; len(ids) > 0 {
							candidates, match = ids, matchFull
							stats.RefsReceiverScoped++
							break
						}
					}
				}
				resolveCandidates(ref, rec.Path, candidates, match, nodes, addEdge, stats)
			}
		}
	}

	// Structural interface satisfaction, after the ref loop because it reads the
	// `defines` edges that loop produced.
	goImplements(nodes, edges, addEdge)

	// Declared dependencies, read from the manifests rather than inferred from
	// imports. Runs after the ref loop so it reuses the external nodes import
	// resolution already synthesized instead of creating rivals to them.
	manifestDeps(w.Root, synthetic, addEdge)

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
	if err := s.PutFile(store.FileRecord{Path: store.SyntheticPath, NodeIDs: ids}, nodeList); err != nil {
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
//
// A tail-only match takes a stricter path. AMBIGUOUS is meant to record genuine
// uncertainty between plausible targets, but a bare-tail hit has already thrown
// away the qualifier that said which one was meant — so its extra candidates are
// not rival readings of the reference, they are unrelated symbols that happen to
// share a method name. Fanning out over them manufactures edges: every
// `err.Error()` call in a repo landed on *every* type with an Error method,
// which on this codebase made three error types the largest, most-connected
// entities in the graph and the most expensive concepts in the exported bundle.
func resolveCandidates(ref model.Ref, siteFile string, candidates []string, match symbolMatch, nodes map[string]model.Node, addEdge func(model.Edge), stats *Stats) {
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
	// The same-file shortcut requires a full match. A tail-only hit reaches
	// here with its qualifier already discarded, so a same-named symbol
	// sharing the file is coincidence, not evidence — and this branch would
	// stamp it EXTRACTED, the highest confidence of all. `defer
	// resp.Body.Close()` in a file that also declares a Close method became
	// "this method calls Close, EXTRACTED"; the qualifier that said the
	// receiver was an http.Response.Body was gone by then. A genuine
	// receiver-qualified call is promoted to matchFull upstream and still
	// takes this branch.
	case len(sameFile) == 1 && match == matchFull:
		emit(sameFile, model.ConfExtracted)
		stats.RefsResolved++
	case len(candidates) == 1:
		emit(candidates, model.ConfInferred)
		stats.RefsResolved++
	case match == matchTail:
		stats.RefsDropped++
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
	// The match kind is irrelevant here: this ladder already demands uniqueness,
	// so it can never fan out over a tail match's extra candidates.
	candidates, _ := lookupSymbol(symbols, lang, name)
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

// symbolMatch records how a symbol lookup hit, because the two ways carry very
// different amounts of evidence.
type symbolMatch int

const (
	matchNone symbolMatch = iota
	// matchFull: the ref's full (possibly qualified) name is a declared symbol.
	matchFull
	// matchTail: only the bare name after the last "." matched, so the
	// qualifier — the receiver or package that says *which* symbol was meant —
	// was discarded to get the hit.
	matchTail
)

// lookupSymbol tries the full (possibly qualified) name, then its bare tail,
// reporting which one hit.
func lookupSymbol(symbols map[string][]string, lang, name string) ([]string, symbolMatch) {
	if ids := symbols[lang+"\x00"+name]; len(ids) > 0 {
		return ids, matchFull
	}
	if i := strings.LastIndex(name, "."); i >= 0 && i+1 < len(name) {
		if ids := symbols[lang+"\x00"+name[i+1:]]; len(ids) > 0 {
			return ids, matchTail
		}
	}
	return nil, matchNone
}

// goStructFieldIndex maps a Go struct's declared name to its field→type map.
//
// Keyed by name rather than node id because the call site names a field's type
// only by name. A name declared by two packages is dropped rather than guessed:
// picking one would attach a call to whichever struct sorted first, which is a
// wrong edge presented with the same confidence as a right one.
func goStructFieldIndex(nodes map[string]model.Node) map[string]map[string]string {
	out := map[string]map[string]string{}
	clashed := map[string]bool{}
	for _, n := range nodes {
		if n.Language != "go" || n.Kind != model.KindStruct || len(n.Fields) == 0 {
			continue
		}
		if _, seen := out[n.Label]; seen {
			clashed[n.Label] = true
			continue
		}
		out[n.Label] = n.Fields
	}
	for name := range clashed {
		delete(out, name)
	}
	return out
}

// goReceiverQualified rewrites a receiver-qualified call into the qualified
// names of the method it may actually name, best first, or nil when it cannot.
//
// It handles the two shapes that carry enough evidence:
//
//	s.field.Method()  inside a method of T, where T declares `field U` → U.Method
//	s.Method()        inside a method of T, s being T's receiver         → T.Method
//
// Both require the head segment to be the enclosing method's receiver variable.
// That check is what keeps this sound: without it, any `x.engine.Evaluate` would
// be read as the enclosing type's `engine` field regardless of what `x` is.
// Deeper chains (`a.b.c.Method`) are not followed — each additional hop needs
// the previous hop's type to itself be a repo struct, and the evidence thins
// faster than the payoff grows.
//
// A list rather than one name because of **method promotion**: `b.Get()` on a
// type that embeds S3Backend names S3Backend.Get, and no declaration on the
// outer type says so. Embedded types are therefore tried after the receiver's
// own type, in declaration order. Promotion is only consulted when the
// receiver's own type has no such method, which is exactly when Go itself
// would promote.
func goReceiverQualified(ref model.Ref, nodes map[string]model.Node, structFields map[string]map[string]string, locals map[string][]model.Local) []string {
	from, ok := nodes[ref.FromID]
	if !ok {
		return nil
	}
	parts := strings.Split(ref.Name, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return nil
	}

	// What the head segment names: the enclosing method's receiver, or a local
	// (parameter, typed var, or a variable holding a constructed value).
	var headType string
	switch {
	case from.Kind == model.KindMethod && from.Recv != "" && parts[0] == from.Recv:
		dot := strings.LastIndexByte(from.Label, '.')
		if dot < 0 {
			return nil
		}
		headType = from.Label[:dot]
	default:
		headType = localTypeAt(locals[ref.FromID], parts[0], ref.Line)
	}
	if headType == "" {
		return nil
	}

	method := parts[len(parts)-1]
	if len(parts) == 3 {
		fieldType, ok := structFields[headType][parts[1]]
		if !ok {
			return nil
		}
		return []string{fieldType + "." + method}
	}
	// x.Method(): x's own type first, then anything it embeds.
	out := []string{headType + "." + method}
	for _, emb := range goEmbeddedTypes(structFields[headType]) {
		out = append(out, emb+"."+method)
	}
	return out
}

// goEmbeddedTypes lists a struct's embedded type names, sorted so the candidate
// order does not vary run to run. Embedded fields are recorded under their own
// type name by the extractor, which is what makes them identifiable here.
func goEmbeddedTypes(fields map[string]string) []string {
	var out []string
	for name, typ := range fields {
		if name == typ {
			out = append(out, typ)
		}
	}
	sort.Strings(out)
	return out
}

// callPrefix mirrors extract.callPrefix: the marker an unresolved local
// carries until its callee's return type is known.
const callPrefix = "()"

// goResolveLocals turns each definition's raw local map into resolved bare type
// names, substituting "()F" for the first result type of F.
//
// The constructor almost always lives in another file, so this cannot happen
// during extraction: `eng := engine.NewEngine(...)` only becomes `eng is an
// Engine` once NewEngine's node is in hand. A callee that is ambiguous, absent,
// or returns nothing named is dropped rather than guessed — the variable simply
// stays untyped, which is the pre-existing behaviour.
func goResolveLocals(nodes map[string]model.Node, symbols map[string][]string) map[string][]model.Local {
	// Bare type names are not node ids; resolve a callee to its Returns.
	returnsOf := func(callee string) string {
		ids, _ := lookupSymbol(symbols, "go", callee)
		if len(ids) != 1 {
			return "" // ambiguous or unknown: no guess
		}
		return nodes[ids[0]].Returns
	}
	out := map[string][]model.Local{}
	for id, n := range nodes {
		if n.Language != "go" || len(n.Locals) == 0 {
			continue
		}
		var resolved []model.Local
		for _, l := range n.Locals {
			if callee, ok := strings.CutPrefix(l.Type, callPrefix); ok {
				l.Type = returnsOf(callee)
			}
			if l.Type != "" {
				resolved = append(resolved, l)
			}
		}
		if len(resolved) > 0 {
			out[id] = resolved
		}
	}
	return out
}

// localTypeAt returns the type bound to name at line, honouring shadowing: the
// innermost declaration whose range covers the line wins, and among equally
// tight ranges the latest declaration at or before the line does.
//
// "Innermost" is the narrowest covering range because Go scopes nest — an inner
// block's range is strictly inside its parent's.
func localTypeAt(locals []model.Local, name string, line int) string {
	best := ""
	bestSpan, bestStart := 0, 0
	for _, l := range locals {
		if l.Name != name || l.Start > line || l.End < line {
			continue
		}
		span := l.End - l.Start
		switch {
		case best == "",
			span < bestSpan,                         // tighter scope: inner wins
			span == bestSpan && l.Start > bestStart: // same scope: later declaration wins
			best, bestSpan, bestStart = l.Type, span, l.Start
		}
	}
	return best
}

// goPackageDirs maps the conventional package qualifier of each of a Go file's
// imports to the repo-relative directory it resolves to.
//
// This is what lets a qualified call keep its qualifier: `store.Open` can be
// scoped to the one package named `store` that this file actually imports,
// instead of falling back to a bare-name lookup that would match every `Open`
// in the repo. Aliased imports are not captured by the extractor, so they
// simply do not scope — the call falls through to the tail rule.
func goPackageDirs(rec store.FileRecord, goModule string, dirHasFiles map[string]bool) map[string]string {
	if goModule == "" {
		return nil
	}
	var dirs map[string]string
	for _, ref := range rec.Refs {
		if ref.Relation != model.RelImports {
			continue
		}
		rel, ok := strings.CutPrefix(ref.Name, goModule)
		if !ok {
			continue // third-party or stdlib: not in this repo
		}
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || !dirHasFiles[rel] {
			continue
		}
		qual := rel
		if i := strings.LastIndex(qual, "/"); i >= 0 {
			qual = qual[i+1:]
		}
		if dirs == nil {
			dirs = map[string]string{}
		}
		dirs[qual] = rel
	}
	return dirs
}

// goImplements infers `implements` edges by comparing method sets.
//
// Go has no syntax that names an implementor: satisfaction is structural, so
// unlike TypeScript's `implements` clause there is nothing for an extractor to
// read. The evidence is entirely in the two method sets, which only exist after
// resolution has reversed the `defines` refs into type→method edges — hence a
// post-pass rather than an extractor rule.
//
// Under-reporting is deliberate where it happens. Methods promoted from an
// embedded struct are not tracked, and an interface that embeds another
// interface contributes no members, so both cases simply produce no edge. The
// alternative — assuming the unseen half matches — would claim relationships
// that are not there, and an absent edge is a far cheaper error than a false one.
func goImplements(nodes map[string]model.Node, edges map[string]model.Edge, addEdge func(model.Edge)) {
	methods := map[string]map[string]bool{} // type id → bare method names
	for _, e := range edges {
		if e.Relation != model.RelDefines {
			continue
		}
		m, ok := nodes[e.Target]
		if !ok {
			continue
		}
		name := m.Label
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if methods[e.Source] == nil {
			methods[e.Source] = map[string]bool{}
		}
		methods[e.Source][name] = true
	}

	var ifaces, types []string
	for id, n := range nodes {
		// An interface with no listed members is either `any` or embeds
		// something unseen; either way every type would "satisfy" it.
		if n.Language == "go" && n.Kind == model.KindInterface && len(n.Members) > 0 && !n.External {
			ifaces = append(ifaces, id)
		}
	}
	for id := range methods {
		if n := nodes[id]; n.Language == "go" && !n.External {
			types = append(types, id)
		}
	}
	// Sorted so the edge set never depends on map iteration order.
	sort.Strings(ifaces)
	sort.Strings(types)

	for _, ifaceID := range ifaces {
		iface := nodes[ifaceID]
		for _, typeID := range types {
			if typeID == ifaceID {
				continue
			}
			if !coversMethods(methods[typeID], iface.Members) {
				continue
			}
			t := nodes[typeID]
			addEdge(model.Edge{
				Source: typeID, Target: ifaceID,
				Relation: model.RelImplements, Confidence: model.ConfInferred,
				File: t.File, Line: t.Span.StartLine,
			})
		}
	}
}

// coversMethods reports whether have contains every name in want.
func coversMethods(have map[string]bool, want []string) bool {
	if len(have) == 0 {
		return false
	}
	for _, name := range want {
		if !have[name] {
			return false
		}
	}
	return true
}

// testPrefixMatch resolves a test-name subject by its longest CamelCase prefix
// that names a real symbol.
//
// Exact matching only catches TestFoo and TestType_Method, but the dominant Go
// convention states the scenario in the name — TestWriteIndexFilesStampsOkfVersionAtRootOnly
// tests WriteIndexFiles. Without this those tests contribute no `tests` edge at
// all and appear only as ordinary callers, which is precisely the distinction
// the relation exists to draw.
//
// Longest-first is what keeps it honest: a shorter prefix is a weaker claim, and
// stopping at the first hit means TestWriteIndexFiles… resolves to
// WriteIndexFiles rather than to a hypothetical Write. Prefixes only break at an
// upper-case boundary, so no match is ever made on half a word.
func testPrefixMatch(symbols map[string][]string, nodes map[string]model.Node, lang, name string) ([]string, symbolMatch) {
	bounds := camelBoundaries(name)
	for i := len(bounds) - 1; i >= 0; i-- {
		prefix := name[:bounds[i]]
		// Short prefixes are common words that match something generic rather
		// than the subject: TestRunIndexMd would otherwise claim to test "Run",
		// and TestFileCommandsAreRegistered to test "File".
		if len(prefix) < minTestPrefix {
			continue
		}
		// The convention capitalizes the subject's first letter regardless of
		// its export status, so runInit becomes TestRunInit. Trying the
		// lower-cased spelling is what lets the longest — and therefore most
		// specific — prefix win before a short generic one can.
		for _, cand := range []string{prefix, lowerFirst(prefix)} {
			if ids := rejectTests(symbols[lang+"\x00"+cand], nodes); len(ids) > 0 {
				// A prefix is weaker evidence than the whole name, so it never
				// fans out: the tail tier drops anything it cannot pin down.
				return ids, matchTail
			}
		}
	}
	return nil, matchNone
}

// minTestPrefix is the shortest subject name a test-name prefix may claim.
const minTestPrefix = 5

// lowerFirst lower-cases an ASCII initial.
func lowerFirst(s string) string {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return s
	}
	return string(s[0]+('a'-'A')) + s[1:]
}

// camelBoundaries returns the indices at which an upper-case letter starts a new
// word, excluding index 0.
func camelBoundaries(s string) []int {
	var out []int
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			out = append(out, i)
		}
	}
	return out
}

// rejectTests drops candidates declared in test files.
func rejectTests(candidates []string, nodes map[string]model.Node) []string {
	var out []string
	for _, id := range candidates {
		if !nodes[id].Test {
			out = append(out, id)
		}
	}
	return out
}

// scopeToPackage narrows candidates to those declared in dir. Returns nil when
// nothing survives, so the caller can fall back rather than drop the ref.
func scopeToPackage(candidates []string, nodes map[string]model.Node, dir string) []string {
	var out []string
	for _, id := range candidates {
		if f := nodes[id].File; f != "" && dirOf(f) == dir {
			out = append(out, id)
		}
	}
	return out
}

// splitLineAnchor separates a "path#L42" reference into its path and line.
// Returns line 0 when there is no anchor.
func splitLineAnchor(name string) (string, int) {
	i := strings.LastIndex(name, "#L")
	if i < 0 {
		return name, 0
	}
	line, err := strconv.Atoi(name[i+2:])
	if err != nil || line < 1 {
		return name, 0
	}
	return name[:i], line
}

// enclosingNode picks the declaration among ids whose span covers line, taking
// the tightest one so a method inside a type wins over the type. Returns "" when
// no declaration covers the line — the caller then keeps the file node, which is
// the honest answer for a link into a comment or a blank stretch.
func enclosingNode(ids []string, nodes map[string]model.Node, line int) string {
	best, bestSize := "", 0
	for _, id := range ids {
		n, ok := nodes[id]
		if !ok || n.Span.StartLine == 0 || !model.Indexable(n) {
			continue
		}
		end := n.Span.EndLine
		if end == 0 {
			end = n.Span.StartLine
		}
		if line < n.Span.StartLine || line > end {
			continue
		}
		if size := end - n.Span.StartLine; best == "" || size < bestSize {
			best, bestSize = id, size
		}
	}
	return best
}

// goPackageQualifier returns the package part of a `pkg.Name` reference. A Go
// package-qualified call always has exactly one dot, so anything else (a method
// chain, a field access) is not a package reference.
func goPackageQualifier(name string) string {
	pkg, rest, ok := strings.Cut(name, ".")
	if !ok || pkg == "" || rest == "" || strings.Contains(rest, ".") {
		return ""
	}
	return pkg
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
	if !model.Indexable(n) || n.Label == "" {
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
