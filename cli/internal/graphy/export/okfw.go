// Package export writes graphy's graph into an OKF Wiki bundle.
//
// The mapping keeps okfw pure: every emitted file is an ordinary OKF concept
// (vanilla frontmatter, vanilla markdown links) created through the shared
// cli/internal/bundle API. Communities become parent concepts; significant
// nodes become child concepts whose bodies are entity cards linking other
// exported concepts. Re-exports are idempotent: the store's exports bucket
// pins each graph key to its concept id and file, so ids and filenames are
// stable across runs.
package export

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
	"okfw/cli/internal/graphdb"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
	"okfw/cli/internal/index"
)

// OkfwOptions configures an export into a bundle.
type OkfwOptions struct {
	Bundle          *config.Config
	Project         string // top folder under graphy/ (default: repo dir name)
	MinDegree       int    // node significance threshold (default 3)
	CommunitiesOnly bool   // skip per-node concepts
	KeepStale       bool   // keep concepts whose graph nodes vanished
	RepoURL         string // optional web base (e.g. https://github.com/org/repo) for clickable resource permalinks
	Generator       string // stamped into the graphy frontmatter block (e.g. "graphy 0.1.0")
	// NoRationale skips rationale concepts entirely.
	NoRationale bool
	// RationaleMarkers selects which markers reach the bundle. Defaults to
	// DefaultRationaleMarkers — deliberately narrower than what extraction
	// captures, since a repo with 800 TODOs should not dump 800 concepts into
	// a wiki by default. The rest stay queryable via graphy and MCP.
	RationaleMarkers []string
}

// DefaultRationaleMarkers are the markers exported as concepts by default.
var DefaultRationaleMarkers = []string{"WHY", "RATIONALE", "DECISION", "ADR"}

// OkfwResult summarizes an export.
type OkfwResult struct {
	Created   int
	Updated   int
	Unchanged int // skipped: rendered content identical to what's on disk
	Renamed   int // adopted from a vanished node via rename/move detection
	Recovered int // adopted from markers after a lost exports mapping
	Deleted   int
	Dir       string // bundle-relative concept directory
}

// communityKeyPrefix namespaces community entries in the exports bucket so
// they cannot collide with node ids.
const communityKeyPrefix = "\x00community:"

// item is one concept to emit (a community or a node).
type item struct {
	key         string // exports-bucket key
	rid         string // recovery id written to frontmatter/marker (node id or "community:<n>")
	title       string
	conceptType string // OKF free-form type: "code-<entity>" / "code-community"
	description string
	language    string
	entity      string // node kind or "community"
	tags        []string
	node        *model.Node      // nil for communities
	community   *store.Community // nil for nodes
	// parentKey overrides the community parent with another item's key. A
	// rationale prefers to hang off the entity it explains.
	parentKey string
}

// markerRe matches the machine-readable node marker embedded in concept
// bodies; it rebuilds the exports mapping when .graphy/graph.db is lost.
var markerRe = regexp.MustCompile(`<!-- graphy:node (\S+) -->`)

// ToOkfw exports the graph into the bundle.
func ToOkfw(root string, s *store.Store, opts OkfwOptions) (*OkfwResult, error) {
	if opts.MinDegree <= 0 {
		opts.MinDegree = 3
	}
	if opts.Project == "" {
		opts.Project = filepath.Base(root)
	}
	cfg := opts.Bundle
	dir := "graphy/" + bundle.Slugify(opts.Project)
	res := &OkfwResult{Dir: dir}

	nodes := map[string]model.Node{}
	if err := s.EachNode(func(n model.Node) error {
		nodes[n.ID] = n
		return nil
	}); err != nil {
		return nil, err
	}

	commit := gitCommit(root)
	items, nodeCommunity, err := selectItems(s, nodes, opts)
	if err != nil {
		return nil, err
	}

	// Pass 1: ensure every item has a concept file; collect key → placement.
	// Resolution order for an item with no live export record:
	//   1. recovery scan — the mapping was lost (.graphy/ deleted) but the
	//      concept exists with the same embedded node identity
	//   2. rename/move adoption — a vanished node's concept carries over so
	//      refactors keep their concept id (and history)
	//   3. create a fresh concept
	placed := map[string]placement{}
	var recovered map[string]placement // rid → placement; scanned lazily
	var pending []int                  // item indices still needing a concept
	for i, it := range items {
		rec, err := s.GetExport(it.key)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			if _, statErr := os.Stat(filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))); statErr == nil {
				placed[it.key] = placement{rec.ConceptID, rec.RelPath}
				continue
			}
		}
		if recovered == nil {
			recovered = scanExisting(cfg, dir)
		}
		if p, ok := recovered[it.rid]; ok {
			if err := s.PutExport(it.key, store.ExportRecord{ConceptID: p.conceptID, RelPath: p.relPath}); err != nil {
				return nil, err
			}
			placed[it.key] = p
			res.Recovered++
			continue
		}
		pending = append(pending, i)
	}

	if len(pending) > 0 {
		adopted, err := adoptRenames(cfg, s, items, pending, placed, res)
		if err != nil {
			return nil, err
		}
		remaining := pending[:0]
		for _, i := range pending {
			if !adopted[items[i].key] {
				remaining = append(remaining, i)
			}
		}
		pending = remaining
	}

	created := map[string]bool{}
	for _, i := range pending {
		it := items[i]
		absPath, id, err := bundle.CreateConcept(cfg, bundle.Spec{
			Title:       it.title,
			Project:     opts.Project,
			Dir:         dir,
			Tags:        it.tags,
			Description: it.description,
		})
		if err != nil {
			return nil, fmt.Errorf("creating concept for %s: %w", it.title, err)
		}
		rel, err := filepath.Rel(cfg.RootPath, absPath)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if err := s.PutExport(it.key, store.ExportRecord{ConceptID: id, RelPath: rel}); err != nil {
			return nil, err
		}
		placed[it.key] = placement{id, rel}
		created[rel] = true
		res.Created++
	}

	// Pass 2: render every concept in full — frontmatter (preserving id and
	// created) and body — with links now that all placements are known.
	//
	// Change detection: the file is first re-rendered with its PREVIOUS
	// timestamp and commit; a byte-identical result means nothing meaningful
	// changed, so the write and index/graph syncs are skipped entirely and
	// `timestamp` keeps its OKF meaning of "last meaningful change" (with
	// graphy.commit pinning the repo state that change was derived from).
	linkTo := func(key string) string {
		if p, ok := placed[key]; ok {
			return "./" + p.relPath[strings.LastIndex(p.relPath, "/")+1:]
		}
		return ""
	}
	var touched []string
	for _, it := range items {
		p := placed[it.key]
		abs := filepath.Join(cfg.RootPath, filepath.FromSlash(p.relPath))
		existing, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		prev, err := config.ParseFrontmatter(existing)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", p.relPath, err)
		}
		parentID := ""
		// A rationale prefers the entity it explains; that entity is only a
		// valid parent if it was exported too, so fall through to the
		// community otherwise (which is what ADRs, explaining many entities,
		// always do). Never leave a dangling parent: the CUE schema requires
		// a real concept id.
		if it.parentKey != "" {
			if p, ok := placed[it.parentKey]; ok {
				parentID = p.conceptID
			}
		}
		if parentID == "" && it.node != nil {
			if c, ok := nodeCommunity[it.node.ID]; ok {
				if cp, ok := placed[communityKey(c)]; ok {
					parentID = cp.conceptID
				}
			}
		}
		var body string
		switch {
		case it.community != nil:
			body = communityBody(it, nodes, linkTo)
		case it.node != nil && it.node.Kind == model.KindRationale:
			body = rationaleBody(s, it, nodes, linkTo)
		default:
			body = nodeBody(s, it, nodes, linkTo)
		}
		if !prev.Timestamp.IsZero() {
			prevCommit := commit
			if prev.Graphy != nil && prev.Graphy.Commit != "" {
				prevCommit = prev.Graphy.Commit
			}
			candidate := renderConcept(prev, it, opts, parentID, prevCommit, prev.Timestamp, body)
			if bytes.Equal(candidate, existing) {
				res.Unchanged++
				continue
			}
		}
		content := renderConcept(prev, it, opts, parentID, commit, time.Now(), body)
		if err := os.WriteFile(abs, content, 0644); err != nil {
			return nil, err
		}
		touched = append(touched, p.relPath)
		if !created[p.relPath] {
			res.Updated++
		}
	}

	// Stale cleanup: export records whose graph key vanished this run.
	if !opts.KeepStale {
		want := map[string]bool{}
		for _, it := range items {
			want[it.key] = true
		}
		type stale struct {
			key string
			rec store.ExportRecord
		}
		var gone []stale
		if err := s.EachExport(func(key string, rec store.ExportRecord) error {
			if !want[key] {
				gone = append(gone, stale{key, rec})
			}
			return nil
		}); err != nil {
			return nil, err
		}
		for _, g := range gone {
			if err := removeConcept(cfg, g.rec); err != nil {
				return nil, err
			}
			if err := s.DeleteExport(g.key); err != nil {
				return nil, err
			}
			res.Deleted++
		}
	}

	if err := syncBundle(cfg, touched); err != nil {
		return nil, err
	}
	return res, nil
}

// selectItems picks the communities and significant nodes to export, ordered
// deterministically (communities first).
func selectItems(s *store.Store, nodes map[string]model.Node, opts OkfwOptions) ([]item, map[string]int, error) {
	var items []item
	nodeCommunity := map[string]int{}

	var comms []store.Community
	if err := s.EachCommunity(func(c store.Community) error {
		for _, id := range c.NodeIDs {
			nodeCommunity[id] = c.ID
		}
		if len(c.NodeIDs) >= 2 && c.Label != "" {
			comms = append(comms, c)
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	sort.Slice(comms, func(i, j int) bool { return comms[i].ID < comms[j].ID })
	for i := range comms {
		c := comms[i]
		items = append(items, item{
			key:         communityKey(c.ID),
			rid:         fmt.Sprintf("community:%d", c.ID),
			title:       "Community: " + c.Label,
			conceptType: "code-community",
			description: fmt.Sprintf("graphy community of %d code entities", len(c.NodeIDs)),
			entity:      "community",
			tags:        []string{"graphy", "graphy/community"},
			community:   &c,
		})
	}

	if !opts.CommunitiesOnly {
		wantMarker := markerFilter(opts)
		var selected []model.Node
		for _, n := range nodes {
			if n.External || n.Kind == model.KindFile || n.Kind == model.KindHeading {
				continue
			}
			if n.Kind == model.KindRationale {
				// MinDegree is a significance proxy for code entities; for a
				// rationale it is meaningless (always ~2). Marker selection
				// takes its place.
				if opts.NoRationale || !wantMarker(n.ID) {
					continue
				}
			} else if n.Degree < opts.MinDegree {
				continue
			}
			selected = append(selected, n)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
		for i := range selected {
			n := selected[i]
			if n.Kind == model.KindRationale {
				items = append(items, rationaleItem(s, n))
				continue
			}
			desc := n.Doc
			if desc == "" {
				desc = n.Signature
			}
			items = append(items, item{
				key:         n.ID,
				rid:         n.ID,
				title:       n.Label,
				conceptType: "code-" + string(n.Kind),
				description: desc,
				language:    n.Language,
				entity:      string(n.Kind),
				tags:        []string{"graphy", "graphy/" + n.Language, "graphy/" + string(n.Kind)},
				node:        &n,
			})
		}
	}
	return items, nodeCommunity, nil
}

// markerFilter reports whether a rationale node id carries an exported marker.
func markerFilter(opts OkfwOptions) func(nodeID string) bool {
	want := opts.RationaleMarkers
	if len(want) == 0 {
		want = DefaultRationaleMarkers
	}
	set := make(map[string]bool, len(want))
	for _, m := range want {
		if m = strings.ToUpper(strings.TrimSpace(m)); m != "" {
			set[m] = true
		}
	}
	return func(nodeID string) bool {
		_, marker, _, ok := model.ParseRationaleQual(qualOfNodeID(nodeID))
		return ok && set[marker]
	}
}

// rationaleItem builds the concept for one rationale node. It parents to the
// entity the rationale explains when that entity was itself exported, falling
// back to the community parent otherwise (which is what ADRs, explaining many
// entities, always do).
func rationaleItem(s *store.Store, n model.Node) item {
	_, marker, _, _ := model.ParseRationaleQual(qualOfNodeID(n.ID))
	title := marker + ": " + truncateTitle(n.Doc)
	if marker == "ADR" {
		title = "ADR: " + truncateTitle(n.Doc)
	}
	parentKey := ""
	if out, err := s.OutEdges(n.ID); err == nil {
		var owners []string
		for _, e := range out {
			if e.Relation == model.RelExplains {
				owners = append(owners, e.Target)
			}
		}
		if len(owners) == 1 {
			parentKey = owners[0]
		}
	}
	return item{
		key:         n.ID,
		rid:         n.ID,
		title:       title,
		conceptType: "code-rationale",
		description: n.Doc,
		language:    n.Language,
		entity:      string(model.KindRationale),
		tags: []string{"graphy", "graphy/" + n.Language, "graphy/rationale",
			"graphy/rationale/" + strings.ToLower(marker)},
		node:      &n,
		parentKey: parentKey,
	}
}

// maxTitleRunes caps a rationale title. bundle.Slugify has no length cap and
// NoteFilename concatenates the whole slug, so an untruncated prose first line
// would produce a filename past the 255-byte limit. Truncation is on a word
// boundary and deterministic, so the filename stays stable.
const maxTitleRunes = 60

func truncateTitle(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= maxTitleRunes {
		return s
	}
	cut := string(r[:maxTitleRunes])
	if i := strings.LastIndexByte(cut, ' '); i > maxTitleRunes/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-")
}

func communityKey(id int) string { return fmt.Sprintf("%s%d", communityKeyPrefix, id) }

// placement locates an exported concept inside the bundle.
type placement struct {
	conceptID string
	relPath   string
}

// resourceFor renders the OKF resource URI for a node: a web permalink pinned
// to the exported commit when a repo URL is configured, else the repo-relative
// source location.
func resourceFor(n model.Node, repoURL, commit string) string {
	if n.File == "" {
		return ""
	}
	line := n.Span.StartLine
	if line < 1 {
		line = 1
	}
	if repoURL != "" && commit != "" {
		return fmt.Sprintf("%s/blob/%s/%s#L%d", strings.TrimSuffix(repoURL, "/"), commit, n.File, line)
	}
	return fmt.Sprintf("%s#L%d", n.File, line)
}

// gitCommit returns the repo's HEAD commit hash ("" outside git).
func gitCommit(root string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// renderConcept produces the concept file in full: frontmatter in okfw's
// quoted house style (preserving the existing id and created stamp) plus the
// body and the machine-readable node marker. The caller controls ts and
// commit so it can render both the "did anything change?" candidate (previous
// stamps) and the final content (fresh stamps).
func renderConcept(prev *config.Concept, it item, opts OkfwOptions, parentID, commit string, ts time.Time, body string) []byte {
	created := prev.Created
	if created.IsZero() {
		created = time.Now()
	}
	resource := ""
	if it.node != nil {
		resource = resourceFor(*it.node, opts.RepoURL, commit)
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %q\n", prev.ID)
	fmt.Fprintf(&b, "title: %q\n", it.title)
	fmt.Fprintf(&b, "type: %q\n", it.conceptType)
	if it.description != "" {
		fmt.Fprintf(&b, "description: %q\n", it.description)
	}
	if resource != "" {
		fmt.Fprintf(&b, "resource: %q\n", resource)
	}
	if opts.Project != "" {
		fmt.Fprintf(&b, "project: %q\n", opts.Project)
	}
	b.WriteString("tags:\n")
	for _, tag := range it.tags {
		fmt.Fprintf(&b, "  - %q\n", tag)
	}
	fmt.Fprintf(&b, "created: %q\n", created.Format(time.RFC3339))
	fmt.Fprintf(&b, "timestamp: %q\n", ts.Format(time.RFC3339))
	if parentID != "" {
		fmt.Fprintf(&b, "parent: %q\n", parentID)
	}
	b.WriteString("graphy:\n")
	fmt.Fprintf(&b, "  node_id: %q\n", it.rid)
	if it.language != "" {
		fmt.Fprintf(&b, "  language: %q\n", it.language)
	}
	fmt.Fprintf(&b, "  entity: %q\n", it.entity)
	if commit != "" {
		fmt.Fprintf(&b, "  commit: %q\n", commit)
	}
	if opts.Generator != "" {
		fmt.Fprintf(&b, "  generator: %q\n", opts.Generator)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", it.title)
	b.WriteString(strings.TrimRight(body, "\n"))
	fmt.Fprintf(&b, "\n\n<!-- graphy:node %s -->\n", it.rid)
	return b.Bytes()
}

// staleConcept is an export record whose graph node vanished this run,
// decomposed for rename matching.
type staleConcept struct {
	key  string // old exports-bucket key
	rec  store.ExportRecord
	lang string
	kind string
	file string
	qual string
}

// adoptRenames matches pending new items against stale concepts so renamed or
// moved entities keep their concept identity (id, created, history). Two
// conservative rules, each requiring a unique 1:1 pairing:
//
//   - move:   same language+kind+qualified-name, different file
//   - rename: same language+kind+file, different name — only when that file
//     has exactly one vanished and exactly one new entity of that kind
//
// Adopted concepts are re-keyed in the exports bucket and their files renamed
// to the new title's slug (keeping the concept id's shortid), so filenames
// stay truthful. Returns the set of adopted item keys.
func adoptRenames(cfg *config.Config, s *store.Store, items []item, pending []int, placed map[string]placement, res *OkfwResult) (map[string]bool, error) {
	adopted := map[string]bool{}

	// Collect stale, parseable node concepts (communities never adopt).
	want := map[string]bool{}
	for _, it := range items {
		want[it.key] = true
	}
	var stale []*staleConcept
	if err := s.EachExport(func(key string, rec store.ExportRecord) error {
		if want[key] || strings.HasPrefix(key, communityKeyPrefix) {
			return nil
		}
		if _, err := os.Stat(filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))); err != nil {
			return nil // file already gone — nothing to adopt
		}
		lang, kind, file, qual, ok := parseNodeID(key)
		if !ok {
			return nil
		}
		stale = append(stale, &staleConcept{key: key, rec: rec, lang: lang, kind: kind, file: file, qual: qual})
		return nil
	}); err != nil {
		return nil, err
	}
	if len(stale) == 0 {
		return adopted, nil
	}

	// Candidate new nodes among the pending items.
	type newEntry struct {
		idx  int
		item item
	}
	var fresh []newEntry
	for _, i := range pending {
		if items[i].node != nil {
			fresh = append(fresh, newEntry{idx: i, item: items[i]})
		}
	}

	pair := func(groupStale func(*staleConcept) string, groupNew func(item) string) map[*staleConcept]newEntry {
		sGroups := map[string][]*staleConcept{}
		for _, sc := range stale {
			if sc != nil {
				sGroups[groupStale(sc)] = append(sGroups[groupStale(sc)], sc)
			}
		}
		nGroups := map[string][]newEntry{}
		for _, ne := range fresh {
			if !adopted[ne.item.key] {
				nGroups[groupNew(ne.item)] = append(nGroups[groupNew(ne.item)], ne)
			}
		}
		out := map[*staleConcept]newEntry{}
		for g, scs := range sGroups {
			if len(scs) == 1 && len(nGroups[g]) == 1 && scs[0] != nil {
				out[scs[0]] = nGroups[g][0]
			}
		}
		return out
	}
	apply := func(matches map[*staleConcept]newEntry) error {
		for sc, ne := range matches {
			if err := adoptOne(cfg, s, sc, ne.item, placed); err != nil {
				return err
			}
			adopted[ne.item.key] = true
			res.Renamed++
			for i := range stale {
				if stale[i] == sc {
					stale[i] = nil
				}
			}
		}
		return nil
	}

	// Rule 1 — file moves: identity is the qualified name.
	moves := pair(
		func(sc *staleConcept) string { return sc.lang + "\x00" + sc.kind + "\x00" + sc.qual },
		func(it item) string {
			_, _, _, qual, _ := parseNodeID(it.rid)
			return it.language + "\x00" + it.entity + "\x00" + qual
		},
	)
	if err := apply(moves); err != nil {
		return nil, err
	}
	// Rule 2 — in-file renames: 1:1 pairing within (lang, kind, file).
	//
	// Rationale is excluded from this rule only. Its qualname already encodes
	// owner, marker and ordinal, so rule 1 handles it correctly; but pairing
	// on (lang, kind, file) alone would match a deleted WHY against a newly
	// added TODO in the same file and adopt the old concept, carrying a stale
	// identity into entirely unrelated reasoning.
	rationaleKind := string(model.KindRationale)
	renames := pair(
		func(sc *staleConcept) string {
			if sc.kind == rationaleKind {
				return "\x00none-stale-" + sc.key
			}
			return sc.lang + "\x00" + sc.kind + "\x00" + sc.file
		},
		func(it item) string {
			if it.node == nil || it.node.Kind == model.KindRationale {
				return "\x00none-" + it.key
			}
			return it.language + "\x00" + it.entity + "\x00" + it.node.File
		},
	)
	if err := apply(renames); err != nil {
		return nil, err
	}
	return adopted, nil
}

// adoptOne re-keys a stale concept to the new item and renames its file to
// match the new title (same concept id, so the shortid suffix is unchanged).
func adoptOne(cfg *config.Config, s *store.Store, sc *staleConcept, it item, placed map[string]placement) error {
	rec := sc.rec
	oldAbs := filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))
	newDir := filepath.Dir(oldAbs)
	newName := bundle.NoteFilename(newDir, it.title, rec.ConceptID)
	if newName != filepath.Base(oldAbs) {
		if err := os.Rename(oldAbs, filepath.Join(newDir, newName)); err != nil {
			return fmt.Errorf("renaming %s: %w", rec.RelPath, err)
		}
		rec.RelPath = filepath.ToSlash(filepath.Join(filepath.Dir(rec.RelPath), newName))
	}
	if err := s.PutExport(it.key, rec); err != nil {
		return err
	}
	if err := s.DeleteExport(sc.key); err != nil {
		return err
	}
	placed[it.key] = placement{rec.ConceptID, rec.RelPath}
	return nil
}

// parseNodeID decomposes "<lang>:<kind>:<file>#<qual>" node ids; ok=false for
// community/synthetic ids that don't follow the shape.
func parseNodeID(id string) (lang, kind, file, qual string, ok bool) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", "", "", false
	}
	rest := parts[2]
	i := strings.LastIndex(rest, "#")
	if i < 0 {
		return "", "", "", "", false
	}
	return parts[0], parts[1], rest[:i], rest[i+1:], true
}

// scanExisting rebuilds rid → placement from the concepts already in the
// export directory, using the frontmatter graphy block (marker comment as
// fallback). This is the recovery path for a lost .graphy/graph.db.
func scanExisting(cfg *config.Config, dir string) map[string]placement {
	out := map[string]placement{}
	root := filepath.Join(cfg.RootPath, filepath.FromSlash(dir))
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || name == "index.md" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			continue
		}
		rid := ""
		if z.Graphy != nil {
			rid = z.Graphy.NodeID
		}
		if rid == "" {
			if m := markerRe.FindSubmatch(content); m != nil {
				rid = string(m[1])
			}
		}
		if rid == "" {
			continue
		}
		out[rid] = placement{conceptID: z.ID, relPath: dir + "/" + name}
	}
	return out
}

// nodeBody renders a node concept's entity card.
func nodeBody(s *store.Store, it item, nodes map[string]model.Node, linkTo func(string) string) string {
	n := *it.node
	var b strings.Builder
	if n.Doc != "" {
		b.WriteString(n.Doc + "\n\n")
	}
	if n.Signature != "" {
		b.WriteString("```" + n.Language + "\n" + n.Signature + "\n```\n\n")
	}
	fmt.Fprintf(&b, "- Kind: %s (%s)\n", n.Kind, n.Language)
	if n.File != "" {
		fmt.Fprintf(&b, "- Location: `%s:%d`\n", n.File, n.Span.StartLine)
	}
	fmt.Fprintf(&b, "- Degree: %d\n", n.Degree)
	writeEdgeSections(&b, s, n.ID, nodes, linkTo)
	return b.String()
}

// writeEdgeSections appends the Outgoing and Incoming edge lists for a node.
func writeEdgeSections(b *strings.Builder, s *store.Store, nodeID string, nodes map[string]model.Node, linkTo func(string) string) {
	out, _ := s.OutEdges(nodeID)
	in, _ := s.InEdges(nodeID)
	writeEdges := func(title string, edges []model.Edge, forward bool) {
		if len(edges) == 0 {
			return
		}
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].Relation != edges[j].Relation {
				return edges[i].Relation < edges[j].Relation
			}
			return edges[i].Source+edges[i].Target < edges[j].Source+edges[j].Target
		})
		b.WriteString("\n## " + title + "\n\n")
		for _, e := range edges {
			otherID := e.Target
			if !forward {
				otherID = e.Source
			}
			other, ok := nodes[otherID]
			label := otherID
			if ok {
				label = other.Label
			}
			if href := linkTo(otherID); href != "" {
				fmt.Fprintf(b, "- %s [%s](%s) (%s)\n", e.Relation, label, href, e.Confidence)
			} else if ok && other.File != "" {
				fmt.Fprintf(b, "- %s %s — `%s:%d` (%s)\n", e.Relation, label, other.File, other.Span.StartLine, e.Confidence)
			} else {
				fmt.Fprintf(b, "- %s %s (%s)\n", e.Relation, label, e.Confidence)
			}
		}
	}
	writeEdges("Outgoing", out, true)
	writeEdges("Incoming", in, false)
}

// rationaleBody renders a rationale concept: the recorded reasoning, where it
// was written, and what it explains.
//
// Deliberately no Degree bullet — it is meaningless for a rationale, and it
// would rewrite the concept whenever an unrelated edge count shifted.
func rationaleBody(s *store.Store, it item, nodes map[string]model.Node, linkTo func(string) string) string {
	n := *it.node
	_, marker, _, _ := model.ParseRationaleQual(qualOfNodeID(n.ID))
	var b strings.Builder
	if n.Body != "" {
		b.WriteString(n.Body + "\n\n")
	} else if n.Doc != "" {
		b.WriteString(n.Doc + "\n\n")
	}
	fmt.Fprintf(&b, "- Marker: %s\n", marker)
	if n.File != "" {
		fmt.Fprintf(&b, "- Source: `%s:%d`\n", n.File, n.Span.StartLine)
	}
	writeEdgeSections(&b, s, n.ID, nodes, linkTo)
	return b.String()
}

// communityBody renders a community concept: a member table linking the
// exported members.
func communityBody(it item, nodes map[string]model.Node, linkTo func(string) string) string {
	c := *it.community
	members := make([]model.Node, 0, len(c.NodeIDs))
	for _, id := range c.NodeIDs {
		if n, ok := nodes[id]; ok {
			members = append(members, n)
		}
	}
	sort.Slice(members, func(i, j int) bool {
		if members[i].Degree != members[j].Degree {
			return members[i].Degree > members[j].Degree
		}
		return members[i].ID < members[j].ID
	})

	var b strings.Builder
	fmt.Fprintf(&b, "A subsystem detected by community clustering: %d entities.\n\n", len(members))
	b.WriteString("| Entity | Kind | Degree | Location |\n|---|---|---:|---|\n")
	for _, n := range members {
		label := n.Label
		if href := linkTo(n.ID); href != "" {
			label = fmt.Sprintf("[%s](%s)", n.Label, href)
		}
		loc := ""
		if n.File != "" {
			loc = fmt.Sprintf("`%s:%d`", n.File, n.Span.StartLine)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", label, n.Kind, n.Degree, loc)
	}
	return b.String()
}

// removeConcept deletes a stale concept file and evicts it from the bundle's
// search index and graph database.
func removeConcept(cfg *config.Config, rec store.ExportRecord) error {
	abs := filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	if idx, err := index.OpenOrCreateIndex(cfg.IndexPath); err == nil {
		_ = idx.Delete(rec.ConceptID)
		idx.Close()
	}
	dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
	if _, err := os.Stat(dbPath); err == nil {
		if gdb, err := graphdb.Open(dbPath); err == nil {
			_ = gdb.PurgeNode(rec.ConceptID)
			gdb.Close()
		}
	}
	return nil
}

// syncBundle batch-indexes the touched concepts: per-file search indexing,
// one index.md regeneration, and per-file graph sync when a graph db exists.
// (bundle.AutoSync per file would regenerate index.md n times.)
func syncBundle(cfg *config.Config, relPaths []string) error {
	if len(relPaths) == 0 {
		if _, err := bundle.WriteIndexFiles(cfg, cfg.RootPath); err != nil {
			return err
		}
		return nil
	}
	for _, rel := range relPaths {
		abs := filepath.Join(cfg.RootPath, filepath.FromSlash(rel))
		if err := bundle.AutoIndex(cfg, abs); err != nil {
			return fmt.Errorf("indexing %s: %w", rel, err)
		}
	}
	if _, err := bundle.WriteIndexFiles(cfg, cfg.RootPath); err != nil {
		return err
	}
	dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	gdb, err := graphdb.Open(dbPath)
	if err != nil {
		return err
	}
	defer gdb.Close()
	for _, rel := range relPaths {
		abs := filepath.Join(cfg.RootPath, filepath.FromSlash(rel))
		if err := gdb.SyncFile(abs, cfg.RootPath); err != nil {
			return fmt.Errorf("graph-syncing %s: %w", rel, err)
		}
	}
	return nil
}

// qualOfNodeID returns just the qualified-name half of a node id.
func qualOfNodeID(id string) string {
	_, _, _, qual, _ := parseNodeID(id)
	return qual
}
