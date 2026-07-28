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
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
	"okfw/cli/internal/graphy/model"
	"okfw/cli/internal/graphy/store"
)

// OkfwOptions configures an export into a bundle.
type OkfwOptions struct {
	Bundle  *config.Config
	Project string // top folder under graphy/ (default: repo dir name)
	// MinDegree is the node significance threshold; 0 exports every eligible
	// node. The CLI defaults it to 3 — the zero value is honoured rather than
	// silently rewritten, so "export everything" is expressible.
	MinDegree       int
	CommunitiesOnly bool   // skip per-node concepts
	KeepStale       bool   // keep concepts whose graph nodes vanished
	RepoURL         string // optional web base (e.g. https://github.com/org/repo) for clickable resource permalinks
	Generator       string // stamped into the graphy frontmatter block (e.g. "graphy 0.1.0")
	// MaxEdges caps how many edges of one relation a concept body lists in each
	// direction; 0 uses DefaultMaxEdges. The remainder is reported with the
	// query that returns it, never silently dropped.
	MaxEdges int
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

// DefaultMaxEdges caps each relation group in a concept body.
const DefaultMaxEdges = 25

// OkfwResult summarizes an export.
//
// The counters answer two independent questions and must not be added together.
// How each concept was *placed*: Created, Renamed, Recovered, Rekeyed, or none
// of these (it was already where the exports bucket said). Whether pass 2 then
// *wrote* it: Updated or Unchanged. An adopted concept is therefore counted once
// under Renamed and once under Updated — which is why Updated used to look
// inflated: it also counted the freshly created ones.
type OkfwResult struct {
	Created   int // new concept files
	Updated   int // existing concepts rewritten because their content changed
	Unchanged int // skipped: rendered content identical to what's on disk
	Renamed   int // adopted from a vanished node via rename/move detection
	Recovered int // re-attached from body markers after a lost exports mapping
	// Relocated counts concepts found elsewhere in the bundle and moved back
	// under the export directory — `okfw move` cannot update the exports
	// bucket, so without this the next export would create a duplicate.
	Relocated int
	// Migrated counts concepts moved because the layout changed underneath
	// them — a flat export converging on the source-mirroring tree. Kept apart
	// from Relocated so a one-time layout migration does not read as the wiki
	// having moved hundreds of concepts by hand.
	Migrated int
	Rekeyed  int // community concept adopted under a new exports key or label
	Deleted   int
	Dir       string // bundle-relative concept directory
	// Skipped counts eligible graph nodes the filters excluded, so a thin
	// export is legible as a filter setting rather than as a broken build.
	Skipped SkipCounts
}

// SkipCounts breaks down what selectItems left out, by reason.
type SkipCounts struct {
	LowDegree      int // below MinDegree
	RationaleMaker int // rationale whose marker is not in RationaleMarkers
	SmallCommunity int // fewer than two members, or no label
}

// Any reports whether anything was skipped.
func (s SkipCounts) Any() bool {
	return s.LowDegree > 0 || s.RationaleMaker > 0 || s.SmallCommunity > 0
}

// communityKeyPrefix namespaces community entries in the exports bucket so
// they cannot collide with node ids.
const communityKeyPrefix = "\x00community:"

// overviewKey is the exports-bucket key for the graph overview concept. Like
// the community prefix it starts with NUL so no node id can collide with it.
const overviewKey = "\x00graph"

// overview marks the graph overview item. It carries no data of its own — its
// body is assembled from the store at render time — but it has to be a distinct
// type so the item switch cannot confuse it with a node or a community.
type overview struct{}

// item is one concept to emit (a community or a node).
type item struct {
	key         string // exports-bucket key
	rid         string // recovery id written to frontmatter/marker (node id or "community:<slug>")
	title       string
	conceptType string // OKF free-form type: "code-<entity>" / "code-community"
	description string
	language    string
	entity      string // node kind or "community"
	tags        []string
	node        *model.Node      // nil for communities and the overview
	community   *store.Community // nil for nodes and the overview
	overview    *overview        // non-nil only for the graph overview concept
	// parentKey overrides the community parent with another item's key. A
	// rationale prefers to hang off the entity it explains.
	parentKey string
}

// markerRe matches the machine-readable node marker embedded in concept
// bodies; it rebuilds the exports mapping when .graphy/graph.db is lost.
var markerRe = regexp.MustCompile(`<!-- graphy:node (\S+) -->`)

// ToOkfw exports the graph into the bundle.
func ToOkfw(root string, s *store.Store, opts OkfwOptions) (*OkfwResult, error) {
	if opts.MinDegree < 0 {
		opts.MinDegree = 0
	}
	if opts.MaxEdges <= 0 {
		opts.MaxEdges = DefaultMaxEdges
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
	items, nodeCommunity, skipped, err := selectItems(s, nodes, opts)
	if err != nil {
		return nil, err
	}
	res.Skipped = skipped

	// Membership digests feed both the adoption matching in pass 1 and the
	// refresh that follows it.
	digests := map[string][]byte{}
	for _, it := range items {
		if it.community != nil {
			digests[it.key] = memberDigest(it.community.NodeIDs)
		}
	}

	// Pass 1a: resolve every item that already has a live concept file, before
	// anything is allowed to adopt. Doing this up front is what keeps adoption
	// safe — see newCommunityAdopter: a record backing a live placement must be
	// off limits, or a second community can adopt a file the first one is
	// already sitting on, renaming it out from under the placement and deleting
	// its record.
	placed := map[string]placement{}
	upToDate := map[string]bool{} // community records already holding the current digest
	live := map[string]bool{}     // exports keys resolved from a live record
	liveRel := map[string]bool{}  // and the files they resolved to
	// The records themselves, so the layout migration below can move a concept
	// without rebuilding a record and dropping a community's membership digest.
	liveRec := map[string]*store.ExportRecord{}
	for _, it := range items {
		rec, err := s.GetExport(it.key)
		if err != nil {
			return nil, err
		}
		if rec == nil {
			continue
		}
		abs := filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))
		content, err := os.ReadFile(abs)
		if os.IsNotExist(err) {
			continue // adoption/recovery/creation will handle it below
		}
		if err != nil {
			return nil, err
		}
		// The record's concept id is a cache of what the file says, and the file
		// is authoritative: a hand edit or a restore-from-git with .graphy/ intact
		// can leave them disagreeing, and children would then get a `parent:`
		// pointing at a concept id that exists nowhere. renderConcept writes the
		// file's own id, so trust that and repair the record.
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", rec.RelPath, err)
		}
		if z.ID != "" && z.ID != rec.ConceptID {
			rec.ConceptID = z.ID
			if err := s.PutExport(it.key, *rec); err != nil {
				return nil, err
			}
		}
		liveRec[it.key] = rec
		placed[it.key] = placement{rec.ConceptID, rec.RelPath}
		live[it.key] = true
		liveRel[rec.RelPath] = true
		if it.community != nil && bytes.Equal(rec.MemberDigest, digests[it.key]) {
			upToDate[it.key] = true
		}
	}

	if err := checkProjectDir(placed, dir); err != nil {
		return nil, err
	}

	// Converge on the current layout. Concepts written by an export that put
	// everything flat sit directly under the export root; move them the first
	// time they are seen, so upgrading needs no re-export from scratch.
	// MoveConcept repairs inbound links at both ends and the concept keeps its
	// id, created stamp and history.
	//
	// Strictly after checkProjectDir: this loop moves concepts between
	// directories, which is exactly what that check refuses to do silently when
	// --project changes. Running it first would have carried out the split it
	// exists to prevent.
	for _, it := range items {
		rec := liveRec[it.key]
		if rec == nil {
			continue
		}
		want := itemDir(dir, it)
		if bundle.RelDir(rec.RelPath) == want {
			continue
		}
		moved, err := relocateInto(cfg, placement{rec.ConceptID, rec.RelPath}, want, it.title)
		if err != nil {
			return nil, err
		}
		delete(liveRel, rec.RelPath)
		rec.RelPath = moved.relPath
		if err := s.PutExport(it.key, *rec); err != nil {
			return nil, err
		}
		placed[it.key] = moved
		liveRel[moved.relPath] = true
		res.Migrated++
	}

	// Pass 1b: place the rest. Resolution order for an item with no live export
	// record:
	//   1. community adoption — an older key scheme, or a changed label
	//   2. recovery scan — the mapping was lost (.graphy/ deleted) but the
	//      concept exists with the same embedded node identity
	//   3. rename/move adoption — a vanished node's concept carries over so
	//      refactors keep their concept id (and history)
	//   4. create a fresh concept
	var recovered map[string]placement // rid → placement; scanned lazily
	var adopter *communityAdopter      // community re-key/relabel matching; loaded lazily
	var pending []int                  // item indices still needing a concept
	for i, it := range items {
		if live[it.key] {
			continue
		}
		var err error
		// A community whose key no longer matches — an older key scheme, or a
		// changed label — adopts its own concept rather than letting the stale
		// sweep delete it and creating a duplicate alongside.
		if it.community != nil {
			if adopter == nil {
				if adopter, err = newCommunityAdopter(cfg, s, live, liveRel); err != nil {
					return nil, err
				}
			}
			if cr, ok := adopter.adopt(it, digests[it.key]); ok {
				rec, err := renameToTitle(cfg, cr.rec, it.title)
				if err != nil {
					return nil, err
				}
				if cr.key != it.key {
					if err := s.DeleteExport(cr.key); err != nil {
						return nil, err
					}
				}
				// The record itself is (re)written by the membership refresh
				// below, which runs for every community.
				placed[it.key] = placement{rec.ConceptID, rec.RelPath}
				res.Rekeyed++
				continue
			}
		}
		if recovered == nil {
			if recovered, err = scanExisting(cfg); err != nil {
				return nil, err
			}
		}
		if p, ok := recovered[it.rid]; ok {
			// A concept found outside the export directory was moved by the
			// wiki side, which cannot update the exports bucket. Bring it back
			// before recording it, or its links to its siblings dangle.
			want := itemDir(dir, it)
			relocated := bundle.RelDir(p.relPath) != want
			if relocated {
				if p, err = relocateInto(cfg, p, want, it.title); err != nil {
					return nil, err
				}
			}
			if err := s.PutExport(it.key, store.ExportRecord{ConceptID: p.conceptID, RelPath: p.relPath}); err != nil {
				return nil, err
			}
			placed[it.key] = p
			liveRel[p.relPath] = true
			if relocated {
				res.Relocated++
			} else {
				res.Recovered++
			}
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
			Dir:         itemDir(dir, it),
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

	// Record each community's current membership. This has to consider every
	// community, not just the ones written above, because the live-record path
	// skips the write entirely — and a membership left stale would make the
	// next relabelling unrecognizable. Records already holding the current
	// digest are skipped, so an unchanged export writes nothing at all, and
	// the rest go out in one transaction rather than one commit apiece.
	updates := map[string]store.ExportRecord{}
	for _, it := range items {
		if it.community == nil || upToDate[it.key] {
			continue
		}
		p := placed[it.key]
		updates[it.key] = store.ExportRecord{
			ConceptID: p.conceptID, RelPath: p.relPath, MemberDigest: digests[it.key],
		}
	}
	if err := s.PutExports(updates); err != nil {
		return nil, err
	}

	// Pass 2: render every concept in full — frontmatter (preserving id and
	// created) and body — with links now that all placements are known.
	//
	// Change detection: the file is first re-rendered with its PREVIOUS
	// timestamp and commit; a byte-identical result means nothing meaningful
	// changed, so the write and index/graph syncs are skipped entirely and
	// `timestamp` keeps its OKF meaning of "last meaningful change" (with
	// graphy.commit pinning the repo state that change was derived from).
	// Wiki-root-relative, not "./<file>.md". Concepts are placed in
	// subdirectories mirroring the source tree, so a sibling reference is no
	// longer a same-directory reference; a bare "./" href would resolve against
	// the linking concept's own directory and silently hit nothing.
	// config.NormalizeTarget treats a leading "/" as the bundle root.
	linkTo := func(key string) string {
		if p, ok := placed[key]; ok {
			return "/" + p.relPath
		}
		return ""
	}
	rOpts := renderOpts{linkTo: linkTo, maxEdges: opts.MaxEdges}
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
			if ck, ok := nodeCommunity[it.node.ID]; ok {
				if cp, ok := placed[ck]; ok {
					parentID = cp.conceptID
				}
			}
		}
		var body string
		switch {
		case it.overview != nil:
			body, err = overviewBody(s, nodes, items, opts, rOpts)
		case it.community != nil:
			body = communityBody(it, nodes, rOpts)
		case it.node != nil && it.node.Kind == model.KindRationale:
			body, err = rationaleBody(s, it, nodes, rOpts)
		default:
			body, err = nodeBody(s, it, nodes, rOpts)
		}
		if err != nil {
			return nil, fmt.Errorf("rendering %s: %w", p.relPath, err)
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
		// renderConcept hand-builds frontmatter, so a bad title, an
		// undeclared graphy field, or a malformed resource would otherwise
		// reach the bundle unnoticed and only surface later as an okfw
		// validation failure on someone else's machine.
		if err := bundle.ValidateGenerated(content); err != nil {
			return nil, fmt.Errorf("%s: %w", p.relPath, err)
		}
		if err := os.WriteFile(abs, content, 0644); err != nil {
			return nil, err
		}
		touched = append(touched, p.relPath)
		// Created counts the file coming into existence; Updated counts an
		// existing concept's content changing. Counting both for a concept
		// created moments ago in the same run double-reports it.
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
		// A file some live item now points at is never stale, whatever key it
		// used to be filed under. Without this, any re-keying (community
		// label-slug migration, say) would delete the very concept it just
		// adopted.
		claimed := map[string]bool{}
		for _, p := range placed {
			claimed[p.relPath] = true
		}
		var gone []stale
		if err := s.EachExport(func(key string, rec store.ExportRecord) error {
			if !want[key] && !claimed[rec.RelPath] {
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

	// Record which bundle this graph was exported into, so the node→concept
	// mapping resolves to a path a reader can actually open. Export records
	// hold a bundle-relative path and nothing else identifies the bundle.
	if err := s.PutMeta(store.MetaBundleKey, cfg.RootPath); err != nil {
		return nil, err
	}

	// Every concept this export owns, changed or not. syncBundle needs the
	// unchanged ones too: the bundle's derived state can be missing for reasons
	// the export knows nothing about.
	all := make([]conceptRef, 0, len(placed))
	for _, p := range placed {
		all = append(all, conceptRef{rel: p.relPath, id: p.conceptID})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].rel < all[j].rel })

	if err := syncBundle(cfg); err != nil {
		return nil, err
	}
	return res, nil
}

// selectItems picks the communities and significant nodes to export, ordered
// deterministically (communities first).
func selectItems(s *store.Store, nodes map[string]model.Node, opts OkfwOptions) ([]item, map[string]string, SkipCounts, error) {
	var items []item
	var skipped SkipCounts
	nodeCommunity := map[string]string{}

	// The overview goes first so every community can parent to it. Without a
	// root the bundle is a thousand sibling concepts and an agent arriving at
	// graphy/<project>/ has nowhere to start; with one the export is a tree —
	// graph, then subsystems, then entities.
	items = append(items, item{
		key:         overviewKey,
		rid:         "graph:" + bundle.Slugify(opts.Project),
		title:       "Code graph: " + opts.Project,
		conceptType: "code-graph",
		description: "Entry point for the " + opts.Project + " code graph.",
		entity:      "graph",
		tags:        []string{"graphy", "graphy/graph"},
		overview:    &overview{},
	})

	var comms []store.Community
	if err := s.EachCommunity(func(c store.Community) error {
		if len(c.NodeIDs) >= 2 && c.Label != "" {
			comms = append(comms, c)
		} else {
			skipped.SmallCommunity++
		}
		return nil
	}); err != nil {
		return nil, nil, skipped, err
	}
	sort.Slice(comms, func(i, j int) bool { return comms[i].ID < comms[j].ID })
	usedSlugs := map[string]bool{}
	for i := range comms {
		c := comms[i]
		slug := uniqueSlug(bundle.Slugify(c.Label), usedSlugs)
		key := communityKey(slug)
		for _, id := range c.NodeIDs {
			nodeCommunity[id] = key
		}
		items = append(items, item{
			key:         key,
			rid:         "community:" + slug,
			title:       "Community: " + c.Label,
			conceptType: "code-community",
			description: fmt.Sprintf("graphy community of %d code entities", len(c.NodeIDs)),
			entity:      "community",
			tags:        []string{"graphy", "graphy/community"},
			community:   &c,
			parentKey:   overviewKey,
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
					skipped.RationaleMaker++
					continue
				}
			} else if n.Degree < opts.MinDegree {
				skipped.LowDegree++
				continue
			}
			selected = append(selected, n)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
		for i := range selected {
			n := selected[i]
			if n.Kind == model.KindRationale {
				it, err := rationaleItem(s, n)
				if err != nil {
					return nil, nil, skipped, err
				}
				items = append(items, it)
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
				tags:        langTags(n.Language, string(n.Kind)),
				node:        &n,
			})
		}
	}
	return items, nodeCommunity, skipped, nil
}

// langTags builds a node's tag set, omitting the language facet when the node
// has no language — synthetic module nodes otherwise picked up a bare "graphy/"
// tag, which the schema accepts and nothing can usefully filter on.
func langTags(language, kind string, extra ...string) []string {
	tags := []string{"graphy"}
	if language != "" {
		tags = append(tags, "graphy/"+language)
	}
	tags = append(tags, "graphy/"+kind)
	return append(tags, extra...)
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
func rationaleItem(s *store.Store, n model.Node) (item, error) {
	_, marker, _, _ := model.ParseRationaleQual(qualOfNodeID(n.ID))
	title := marker + ": " + truncateTitle(n.Doc)
	if marker == "ADR" {
		title = "ADR: " + truncateTitle(n.Doc)
	}
	parentKey := ""
	out, err := s.OutEdges(n.ID)
	if err != nil {
		return item{}, err
	}
	var owners []string
	for _, e := range out {
		if e.Relation == model.RelExplains {
			owners = append(owners, e.Target)
		}
	}
	if len(owners) == 1 {
		parentKey = owners[0]
	}
	return item{
		key:         n.ID,
		rid:         n.ID,
		title:       title,
		conceptType: "code-rationale",
		description: n.Doc,
		language:    n.Language,
		entity:      string(model.KindRationale),
		tags:        langTags(n.Language, "rationale", "graphy/rationale/"+strings.ToLower(marker)),
		node:        &n,
		parentKey:   parentKey,
	}, nil
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

// communityRecord is an existing community export record, with the state
// needed to decide whether a current community is the same subsystem.
type communityRecord struct {
	key     string
	rec     store.ExportRecord
	title   string
	members map[uint64]bool // decoded from rec.MemberDigest
}

// communityAdopter matches communities to concepts they should inherit when
// the exports-bucket key no longer matches — either because the key scheme
// changed, or because the community's label did.
type communityAdopter struct {
	records  []communityRecord
	consumed map[string]bool
}

// newCommunityAdopter loads the community export records eligible for adoption:
// those whose concept file still exists and that no live placement is already
// using. Records for vanished files are skipped; the stale sweep cleans them up.
//
// The live exclusion is load-bearing, not defensive. Both matchers below can
// select a record another item resolved directly — a community that splits
// leaves a successor whose membership still overlaps the original by more than
// half, and two communities can carry byte-identical titles. Adopting such a
// record renames the file the live item is already placed at and deletes the
// other community's record, so pass 2 then fails reading a path that no longer
// exists, after those writes have landed.
func newCommunityAdopter(cfg *config.Config, s *store.Store, live, liveRel map[string]bool) (*communityAdopter, error) {
	a := &communityAdopter{consumed: map[string]bool{}}
	err := s.EachExport(func(key string, rec store.ExportRecord) error {
		if !strings.HasPrefix(key, communityKeyPrefix) || live[key] || liveRel[rec.RelPath] {
			return nil
		}
		content, err := os.ReadFile(filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath)))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			return nil // an unparseable concept is not a candidate, but not fatal either
		}
		a.records = append(a.records, communityRecord{
			key: key, rec: rec, title: z.Title, members: digestSet(rec.MemberDigest),
		})
		return nil
	})
	// bbolt iterates keys in byte order, but sorting explicitly keeps the
	// tie-breaks below independent of the key scheme.
	sort.Slice(a.records, func(i, j int) bool { return a.records[i].key < a.records[j].key })
	return a, err
}

// adopt returns the record a community should inherit, if any.
//
// Two matchers, in order of confidence:
//
//  1. Identical concept title — the label is unchanged, so only the key
//     scheme moved (a bundle written before label-slug keying). Titles are not
//     unique, so records are consumed one per same-titled community.
//
//  2. Membership overlap above communityOverlapThreshold — the label changed
//     but it is the same subsystem, so the concept should follow the members
//     rather than the name.
func (a *communityAdopter) adopt(it item, digest []byte) (communityRecord, bool) {
	for i := range a.records {
		r := &a.records[i]
		if !a.consumed[r.key] && r.title == it.title {
			a.consumed[r.key] = true
			return *r, true
		}
	}
	if it.community == nil {
		return communityRecord{}, false
	}
	best, bestScore := -1, communityOverlapThreshold
	for i := range a.records {
		r := &a.records[i]
		if a.consumed[r.key] || len(r.members) == 0 {
			continue
		}
		score := jaccardDigest(digest, r.members)
		// Strictly greater, and ties broken on key, so the choice never
		// depends on iteration order.
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		return communityRecord{}, false
	}
	a.consumed[a.records[best].key] = true
	return a.records[best], true
}

// memberDigest packs a community's membership into sorted 64-bit FNV-1a hashes
// of its node ids, big-endian.
//
// Hashing rather than storing the ids keeps the record roughly six times
// smaller — node ids average ~60 bytes — without giving up exact set
// semantics. At 64 bits, a collision anywhere in a 100k-node repo has
// probability ~3e-10, and its worst effect would be to shift one similarity
// score by a single member. That exactness is worth protecting: the
// unambiguity argument below is a proof about real set overlap, and a
// probabilistic sketch (MinHash and friends) would reduce it to an estimate
// precisely where it is most sensitive — at the threshold. Matching a 1%
// standard error would also cost ~78KB per community, far more than the exact
// digest it replaces.
//
// Sorting makes the blob canonical, so an unchanged membership compares equal
// byte for byte and its write can be skipped entirely.
func memberDigest(ids []string) []byte {
	seen := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		h := fnv.New64a()
		h.Write([]byte(id))
		seen[h.Sum64()] = struct{}{}
	}
	hashes := make([]uint64, 0, len(seen))
	for h := range seen {
		hashes = append(hashes, h)
	}
	slices.Sort(hashes)
	out := make([]byte, 8*len(hashes))
	for i, h := range hashes {
		binary.BigEndian.PutUint64(out[i*8:], h)
	}
	return out
}

// digestSet decodes a packed digest into a lookup set.
func digestSet(b []byte) map[uint64]bool {
	out := make(map[uint64]bool, len(b)/8)
	for i := 0; i+8 <= len(b); i += 8 {
		out[binary.BigEndian.Uint64(b[i:])] = true
	}
	return out
}

// jaccardDigest is |A∩B| / |A∪B| for a packed digest against a decoded set.
// The digest is already deduplicated, so its length is |A|.
func jaccardDigest(a []byte, b map[uint64]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter, n := 0, 0
	for i := 0; i+8 <= len(a); i += 8 {
		n++
		if b[binary.BigEndian.Uint64(a[i:])] {
			inter++
		}
	}
	union := n + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// communityOverlapThreshold is the share of the union two membership sets must
// have in common for one to count as the same subsystem as the other.
//
// A half is not a tuning knob, it is what makes the match provably
// unambiguous. Communities partition the graph, so both the old records and
// the new communities are internally disjoint; for disjoint sets the overlaps
// against any single opposing set sum to at most 1, so at most one candidate
// on either side can exceed a half. A community that splits down the middle,
// or two that merge, therefore adopt nothing and start fresh — which is the
// honest outcome, because no surviving concept is "the same subsystem".
const communityOverlapThreshold = 0.5

// communityKey is the exports-bucket key for a community, namespaced by its
// label slug.
//
// Keying on the label rather than the detection id is what keeps community
// concepts stable. Community ids are assigned by ordering on each community's
// lexicographically-smallest member, so adding or removing a single node can
// renumber most of the graph — and every renumbering used to delete and
// recreate the affected concepts, losing their ids and repointing `parent:` on
// every child. A label only changes when the community's most prominent member
// does, which is far rarer.
func communityKey(slug string) string { return communityKeyPrefix + slug }

// uniqueSlug disambiguates two communities that name themselves after
// same-named members (say, a `Store` in two packages). Callers must pass
// communities in ascending id order, which — because ids are ordered by
// smallest member — makes the suffix assignment as stable as the ids are.
func uniqueSlug(slug string, used map[string]bool) string {
	if !used[slug] {
		used[slug] = true
		return slug
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s-%d", slug, n)
		if !used[cand] {
			used[cand] = true
			return cand
		}
	}
}

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
	// Every exported concept is kind "code". Without it GetKind falls through
	// the legacy `type` switch — "code-function" matches nothing — and returns
	// "note", so `okfw search --kind note` handed back the entire code export
	// mixed in with the hand-written notes it was asked for.
	b.WriteString("kind: \"code\"\n")
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
			// The file comes from the node id, matching how the stale side is
			// decomposed. Node.File is empty for synthetic nodes (internal
			// package modules, ansible roles, terraform local modules), so
			// reading it here made their group key unmatchable and rule 2 could
			// never fire for them.
			_, _, file, _, ok := parseNodeID(it.rid)
			if !ok {
				return "\x00none-" + it.key
			}
			return it.language + "\x00" + it.entity + "\x00" + file
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
	rec, err := renameToTitle(cfg, sc.rec, it.title)
	if err != nil {
		return err
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

// renameToTitle moves an adopted concept file so its filename reflects the
// title it now carries, keeping the concept id (and so the shortid suffix)
// unchanged. Without this an adopted concept keeps a filename describing what
// it used to be, which reads as a bug long after the fact.
//
// The move goes through bundle.MoveConcept rather than os.Rename so inbound
// links are repaired: a hand-authored concept pointing at a graphy concept would
// otherwise dangle the moment that concept is adopted. Renames are rare by
// construction (an unchanged title returns early), so MoveConcept's bundle walk
// is not on any hot path.
func renameToTitle(cfg *config.Config, rec store.ExportRecord, title string) (store.ExportRecord, error) {
	oldAbs := filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))
	if filenameEncodes(filepath.Base(oldAbs), title, rec.ConceptID) {
		return rec, nil
	}
	newDir := filepath.Dir(oldAbs)
	newName := bundle.NoteFilename(newDir, title, rec.ConceptID)
	if _, err := bundle.MoveConcept(cfg, oldAbs, filepath.Join(newDir, newName)); err != nil {
		return rec, fmt.Errorf("renaming %s: %w", rec.RelPath, err)
	}
	rec.RelPath = filepath.ToSlash(filepath.Join(filepath.Dir(rec.RelPath), newName))
	return rec, nil
}

// filenameEncodes reports whether name is already the "slug--shortid.md" form
// for this title and concept id, i.e. renaming would be a no-op.
//
// This cannot be answered by comparing against bundle.NoteFilename, which was
// the original guard and never fired: NoteFilename skips candidate names that
// exist on disk, and the file being renamed is itself on disk, so it always
// returned the *next* shortid length and every adoption renamed — including the
// legacy community re-key, which is supposed to happen in place.
func filenameEncodes(name, title, conceptID string) bool {
	base, ok := strings.CutSuffix(name, ".md")
	if !ok {
		return false
	}
	slug, short, ok := strings.Cut(base, "--")
	if !ok || slug != bundle.Slugify(title) || short == "" {
		return false
	}
	return strings.HasPrefix(strings.ReplaceAll(conceptID, "-", ""), short)
}

// itemDir is the directory a concept belongs in: the export root followed by
// the source directory of the entity it documents.
//
// A concept's OKF id is its path minus ".md" (section 4), so a flat export gave
// every entity an id like "graphy/<project>/engine-evaluate--019fa597" — an
// opaque handle carrying nothing a reader could use. Mirroring the source tree
// makes the id self-describing and matches `resource:`, so a search hit says
// where the code lives before the concept is opened.
//
// Communities, the graph overview and entities with no source file (externals)
// have no source directory and stay at the export root, which is also where
// the tree's parents belong.
func itemDir(root string, it item) string {
	if it.node == nil || it.node.File == "" {
		return root
	}
	d := path.Dir(filepath.ToSlash(it.node.File))
	if d == "." || d == "/" || d == "" {
		return root
	}
	return root + "/" + d
}

// exportRootOf recovers the export root ("graphy/<project>") from a concept's
// path, now that concepts sit at varying depths beneath it.
func exportRootOf(rel string) string {
	if parts := strings.SplitN(rel, "/", 3); len(parts) >= 2 && parts[0] == "graphy" {
		return parts[0] + "/" + parts[1]
	}
	return bundle.RelDir(rel)
}

// checkProjectDir refuses an export whose live concepts live somewhere other
// than the directory this run would write to.
//
// Changing --project (or the repo directory name that defaults it) moves the
// output directory, but live export records keep pointing into the old one. The
// two halves then render into separate directories while linkTo emits a bare
// "./<file>.md" for every link, so every cross-directory reference silently
// resolves to nothing. Failing with the old name is more useful than half an
// export.
func checkProjectDir(placed map[string]placement, dir string) error {
	for _, p := range placed {
		if old := exportRootOf(p.relPath); old != dir {
			return fmt.Errorf("this bundle's graphy concepts are in %s but this export targets %s: "+
				"re-run with --project matching %s, or delete %s to re-export from scratch",
				old, dir, path.Base(old), old)
		}
	}
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

// scanExisting rebuilds rid → placement from the concepts in the bundle, using
// the frontmatter graphy block (marker comment as fallback). This is the
// recovery path for a lost .graphy/graph.db.
//
// The walk covers the whole bundle, not just the export directory. `okfw move`
// relocates a concept without telling graphy — cmd/okfw imports no graphy
// package and cannot touch the exports bucket — so a directory-scoped scan
// found nothing, adoptRenames skipped it (its record's path no longer stats),
// and the export created a *duplicate*: two files carrying the same
// graphy.node_id, and `okfw search --node-id` returning two hits.
//
// A file that is not a recoverable concept is skipped, but an I/O failure is
// returned: recovery that silently degrades into "found nothing" creates a
// duplicate concept for every entity in the bundle.
func scanExisting(cfg *config.Config) (map[string]placement, error) {
	out := map[string]placement{}
	root := cfg.RootPath
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if bundle.SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || bundle.IsReservedBase(fi.Name()) {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || z.ID == "" {
			return nil // not a concept we can recover from
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
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rid] = placement{conceptID: z.ID, relPath: filepath.ToSlash(rel)}
		return nil
	})
	if os.IsNotExist(err) {
		return map[string]placement{}, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// relocateInto moves a recovered concept back under the export directory.
//
// Recovering it where it lies is not an option: linkTo emits a bare
// "./<file>.md" for every reference, so a concept outside dir is unreachable
// from its siblings and they from it — the same split checkProjectDir refuses
// for a changed --project. MoveConcept repairs inbound links at both ends, and
// the concept keeps its id, its created stamp and its history.
func relocateInto(cfg *config.Config, p placement, dir, title string) (placement, error) {
	absDir := filepath.Join(cfg.RootPath, filepath.FromSlash(dir))
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return p, err
	}
	name := bundle.NoteFilename(absDir, title, p.conceptID)
	from := filepath.Join(cfg.RootPath, filepath.FromSlash(p.relPath))
	to := filepath.Join(absDir, name)
	if from == to {
		return p, nil
	}
	if _, err := bundle.MoveConcept(cfg, from, to); err != nil {
		return p, fmt.Errorf("relocating %s into %s: %w", p.relPath, dir, err)
	}
	return placement{conceptID: p.conceptID, relPath: dir + "/" + name}, nil
}

// nodeBody renders a node concept's entity card.
func nodeBody(s *store.Store, it item, nodes map[string]model.Node, opts renderOpts) (string, error) {
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
	if n.Test {
		b.WriteString("- Test code\n")
	}
	fmt.Fprintf(&b, "- Degree: %d\n", n.Degree)
	if len(n.Members) > 0 {
		fmt.Fprintf(&b, "- Methods: %s\n", strings.Join(n.Members, ", "))
	}
	if n.Kind == model.KindDoc {
		if err := writeOutline(&b, s, n, nodes, opts); err != nil {
			return "", err
		}
	}
	if err := writeEdgeSections(&b, s, n.ID, nodes, opts); err != nil {
		return "", err
	}
	return b.String(), nil
}

// writeOutline renders a document concept's headings, each with what that
// section refers to.
//
// Headings never become concepts of their own — one concept per H2 would bury
// the document it belongs to — but they are where a document's references
// actually hang, so without this a doc concept showed a flat list of unlinked
// heading names and no sign of what any section was about. As an outline the
// same data becomes a table of contents that says which section describes which
// entity, which is the cheapest possible way for an agent to decide what to read.
func writeOutline(b *strings.Builder, s *store.Store, doc model.Node, nodes map[string]model.Node, opts renderOpts) error {
	contains, err := s.OutEdges(doc.ID)
	if err != nil {
		return err
	}
	type section struct {
		node  model.Node
		links []string
	}
	var sections []section
	for _, e := range contains {
		h, ok := nodes[e.Target]
		if !ok || e.Relation != model.RelContains || h.Kind != model.KindHeading {
			continue
		}
		refs, err := s.OutEdges(h.ID)
		if err != nil {
			return err
		}
		sec := section{node: h}
		for _, r := range refs {
			linkID, label, keep := endpoint(r, true, nodes)
			if !keep {
				continue
			}
			if href := opts.linkTo(linkID); href != "" {
				label = fmt.Sprintf("[%s](%s)", label, href)
			}
			sec.links = append(sec.links, label)
		}
		sort.Strings(sec.links)
		if len(sec.links) > opts.maxEdges {
			sec.links = append(sec.links[:opts.maxEdges],
				fmt.Sprintf("… %d more", len(sec.links)-opts.maxEdges))
		}
		sections = append(sections, sec)
	}
	if len(sections) == 0 {
		return nil
	}
	sort.Slice(sections, func(i, j int) bool {
		return sections[i].node.Span.StartLine < sections[j].node.Span.StartLine
	})
	b.WriteString("\n## Outline\n\n")
	for _, sec := range sections {
		fmt.Fprintf(b, "- %s", sec.node.Label)
		if len(sec.links) > 0 {
			fmt.Fprintf(b, " — %s", strings.Join(sec.links, ", "))
		}
		b.WriteString("\n")
	}
	return nil
}

// writeEdgeSections appends the Outgoing and Incoming edge lists for a node.
// A store read failure is returned rather than dropped: swallowing it rendered
// a concept with no edges at all, which is indistinguishable from a leaf node
// and would then be written to disk as the new truth.
//
// Three things keep this readable rather than exhaustive, because a concept
// nobody can afford to read is not documentation:
//
//   - Structural edges to kinds that never become concepts are dropped, not
//     rendered as dead text. A `contains` edge from the file a function lives in
//     restates `resource:`, and on this repository those alone accounted for a
//     fifth of every bullet the export produced.
//   - Edges are grouped by relation and capped, so one hub relation cannot
//     crowd out the rest. Truncation is reported and points at the command that
//     returns the full set.
//   - AMBIGUOUS edges are segregated. Mixed into the list they read exactly like
//     resolved ones while being the least trustworthy thing on the page.
func writeEdgeSections(b *strings.Builder, s *store.Store, nodeID string, nodes map[string]model.Node, opts renderOpts) error {
	out, err := s.OutEdges(nodeID)
	if err != nil {
		return err
	}
	in, err := s.InEdges(nodeID)
	if err != nil {
		return err
	}
	writeEdgeList(b, "Outgoing", out, true, nodes, opts)
	writeEdgeList(b, "Incoming", in, false, nodes, opts)
	return nil
}

// renderOpts carries what body rendering needs beyond the graph itself.
type renderOpts struct {
	// linkTo maps an exports key to a relative href, "" when not exported.
	linkTo func(string) string
	// maxEdges caps each relation group within a direction.
	maxEdges int
}

// endpoint resolves the other end of an edge to what should be shown for it:
// the node it should link to (redirecting where the real endpoint is never a
// concept), the text to display, and whether to render it at all.
func endpoint(e model.Edge, forward bool, nodes map[string]model.Node) (linkID, label string, keep bool) {
	id := e.Target
	if !forward {
		id = e.Source
	}
	n, known := nodes[id]
	if !known {
		return id, id, true
	}
	switch n.Kind {
	case model.KindFile:
		// `contains` from a file restates the concept's own resource: field.
		// Other relations (a file importing a module) still carry information,
		// so they stay as plain text naming the file.
		if e.Relation == model.RelContains {
			return "", "", false
		}
		return "", n.Label, true
	case model.KindHeading:
		// A heading is never a concept, but the document holding it is, and it
		// is what a reader wants to open. Redirect to the doc and keep the
		// heading as the label — this is what makes a doc→code reference
		// navigable from the code side rather than a dead mention.
		if e.Relation == model.RelContains {
			return "", "", false // the doc renders its own outline
		}
		return model.NodeID("markdown", model.KindDoc, n.File, ""), n.Label, true
	}
	return id, n.Label, true
}

// writeEdgeList renders one direction, grouped by relation and capped.
func writeEdgeList(b *strings.Builder, title string, edges []model.Edge, forward bool, nodes map[string]model.Node, opts renderOpts) {
	type bullet struct {
		relation   model.Relation
		text       string
		confidence model.Confidence
		degree     int
	}
	groups := map[model.Relation][]bullet{}
	for _, e := range edges {
		linkID, label, keep := endpoint(e, forward, nodes)
		if !keep {
			continue
		}
		other := nodes[linkID]
		var text string
		switch href := opts.linkTo(linkID); {
		case href != "":
			text = fmt.Sprintf("[%s](%s)", label, href)
		case other.File != "":
			text = fmt.Sprintf("%s — `%s:%d`", label, other.File, other.Span.StartLine)
		default:
			text = label
		}
		groups[e.Relation] = append(groups[e.Relation], bullet{e.Relation, text, e.Confidence, other.Degree})
	}
	if len(groups) == 0 {
		return
	}

	rels := make([]model.Relation, 0, len(groups))
	for r := range groups {
		rels = append(rels, r)
	}
	sort.Slice(rels, func(i, j int) bool { return rels[i] < rels[j] })

	b.WriteString("\n## " + title + "\n")
	for _, rel := range rels {
		list := groups[rel]
		// Most trustworthy first, then most connected: a truncated list should
		// keep the entries a reader would have wanted.
		sort.SliceStable(list, func(i, j int) bool {
			if ci, cj := confRank(list[i].confidence), confRank(list[j].confidence); ci != cj {
				return ci > cj
			}
			if list[i].degree != list[j].degree {
				return list[i].degree > list[j].degree
			}
			return list[i].text < list[j].text
		})
		var solid, ambiguous []bullet
		for _, x := range list {
			if x.confidence == model.ConfAmbiguous {
				ambiguous = append(ambiguous, x)
			} else {
				solid = append(solid, x)
			}
		}
		fmt.Fprintf(b, "\n### %s (%d)\n\n", rel, len(list))
		shown, hidden := solid, 0
		if len(shown) > opts.maxEdges {
			hidden = len(shown) - opts.maxEdges
			shown = shown[:opts.maxEdges]
		}
		for _, x := range shown {
			fmt.Fprintf(b, "- %s (%s)\n", x.text, x.confidence)
		}
		if hidden > 0 {
			fmt.Fprintf(b, "- … %d more — `graphy neighbors <entity> --rel %s`\n", hidden, rel)
		}
		if len(ambiguous) > 0 {
			names := make([]string, 0, len(ambiguous))
			for _, x := range ambiguous {
				names = append(names, x.text)
			}
			if len(names) > opts.maxEdges {
				names = append(names[:opts.maxEdges], fmt.Sprintf("… %d more", len(ambiguous)-opts.maxEdges))
			}
			fmt.Fprintf(b, "- Ambiguous (unresolved between several targets): %s\n", strings.Join(names, ", "))
		}
	}
	return
}

// confRank orders confidences most-trustworthy first.
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

// rationaleBody renders a rationale concept: the recorded reasoning, where it
// was written, and what it explains.
//
// Deliberately no Degree bullet — it is meaningless for a rationale, and it
// would rewrite the concept whenever an unrelated edge count shifted.
func rationaleBody(s *store.Store, it item, nodes map[string]model.Node, opts renderOpts) (string, error) {
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
	if err := writeEdgeSections(&b, s, n.ID, nodes, opts); err != nil {
		return "", err
	}
	return b.String(), nil
}

// maxOverviewHubs is how many entry-point entities the overview names.
const maxOverviewHubs = 15

// overviewBody renders the bundle's front door: how the graph was built, the
// subsystems it found, and where to start reading.
//
// It also carries the one thing about an exported bundle that was previously
// unrecoverable from it — the clustering algorithm, resolution and seed. Every
// community concept is a product of those settings, so a bundle that does not
// state them cannot be reproduced or argued with.
func overviewBody(s *store.Store, nodes map[string]model.Node, items []item, opts OkfwOptions, rOpts renderOpts) (string, error) {
	nodeCount, edgeCount, err := s.Counts()
	if err != nil {
		return "", err
	}
	meta, err := s.GetClusterMeta()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Generated code graph for **%s**. Start here, then follow a subsystem down to the entities it holds.\n\n", opts.Project)

	byKind := map[model.Kind]int{}
	byLang := map[string]int{}
	tests := 0
	for _, n := range nodes {
		byKind[n.Kind]++
		if n.Language != "" {
			byLang[n.Language]++
		}
		if n.Test {
			tests++
		}
	}
	fmt.Fprintf(&b, "- Graph: %d nodes, %d edges\n", nodeCount, edgeCount)
	fmt.Fprintf(&b, "- Languages: %s\n", countList(byLang))
	fmt.Fprintf(&b, "- Entities: %s\n", countList(kindCounts(byKind)))
	if tests > 0 {
		fmt.Fprintf(&b, "- Test code: %d of %d nodes\n", tests, nodeCount)
	}
	if meta.Algorithm != "" {
		fmt.Fprintf(&b, "- Clustering: %s, resolution %g, seed %d\n", meta.Algorithm, meta.Resolution, meta.Seed)
	}
	fmt.Fprintf(&b, "- Exported with `--min-degree %d`", opts.MinDegree)
	if opts.CommunitiesOnly {
		b.WriteString(", communities only")
	}
	if opts.Generator != "" {
		fmt.Fprintf(&b, " by %s", opts.Generator)
	}
	b.WriteString("\n")

	var comms []item
	for _, it := range items {
		if it.community != nil {
			comms = append(comms, it)
		}
	}
	sort.Slice(comms, func(i, j int) bool {
		if a, z := len(comms[i].community.NodeIDs), len(comms[j].community.NodeIDs); a != z {
			return a > z
		}
		return comms[i].title < comms[j].title
	})
	if len(comms) > 0 {
		fmt.Fprintf(&b, "\n## Subsystems (%d)\n\nLargest first.\n\n", len(comms))
		for _, it := range comms {
			label := cell(it.community.Label)
			if href := rOpts.linkTo(it.key); href != "" {
				label = fmt.Sprintf("[%s](%s)", label, href)
			}
			fmt.Fprintf(&b, "- %s — %d entities\n", label, len(it.community.NodeIDs))
		}
	}

	// Entry points are the highest-degree exported entities: whatever else a
	// reader is looking for, these are what the rest of the graph hangs off.
	var hubs []item
	for _, it := range items {
		if it.node != nil && it.node.Kind != model.KindRationale && !it.node.Test {
			hubs = append(hubs, it)
		}
	}
	sort.Slice(hubs, func(i, j int) bool {
		if a, z := hubs[i].node.Degree, hubs[j].node.Degree; a != z {
			return a > z
		}
		return hubs[i].node.ID < hubs[j].node.ID
	})
	if len(hubs) > maxOverviewHubs {
		hubs = hubs[:maxOverviewHubs]
	}
	if len(hubs) > 0 {
		b.WriteString("\n## Entry points\n\nThe most connected entities.\n\n")
		b.WriteString("| Entity | Kind | Degree | Location |\n|---|---|---:|---|\n")
		for _, it := range hubs {
			n := *it.node
			label := cell(n.Label)
			if href := rOpts.linkTo(it.key); href != "" {
				label = fmt.Sprintf("[%s](%s)", label, href)
			}
			loc := ""
			if n.File != "" {
				loc = fmt.Sprintf("`%s:%d`", cell(n.File), n.Span.StartLine)
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", label, n.Kind, n.Degree, loc)
		}
	}
	return b.String(), nil
}

// kindCounts re-keys a kind tally as strings for countList.
func kindCounts(byKind map[model.Kind]int) map[string]int {
	out := make(map[string]int, len(byKind))
	for k, v := range byKind {
		out[string(k)] = v
	}
	return out
}

// countList renders a tally as "12 go, 4 python", largest first.
func countList(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
	}
	return strings.Join(parts, ", ")
}

// maxCommunityRows caps a community concept's member table. Communities are
// whole subsystems — the largest in this repository holds over two hundred
// entities — and a table that long is a listing, not an orientation. The counts
// line above it keeps the shape of the rest visible, and the full membership is
// a query away.
const maxCommunityRows = 40

// communityBody renders a community concept: what it is made of, then its most
// connected members.
func communityBody(it item, nodes map[string]model.Node, opts renderOpts) string {
	c := *it.community
	members := make([]model.Node, 0, len(c.NodeIDs))
	byKind := map[model.Kind]int{}
	tests := 0
	for _, id := range c.NodeIDs {
		if n, ok := nodes[id]; ok {
			members = append(members, n)
			byKind[n.Kind]++
			if n.Test {
				tests++
			}
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

	kinds := make([]model.Kind, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if byKind[kinds[i]] != byKind[kinds[j]] {
			return byKind[kinds[i]] > byKind[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%d %s", byKind[k], k))
	}
	fmt.Fprintf(&b, "- Composition: %s\n", strings.Join(parts, ", "))
	if tests > 0 {
		fmt.Fprintf(&b, "- Test code: %d of %d entities\n", tests, len(members))
	}

	shown := members
	if len(shown) > maxCommunityRows {
		shown = shown[:maxCommunityRows]
	}
	fmt.Fprintf(&b, "\n## Members\n\nMost connected first.\n\n")
	b.WriteString("| Entity | Kind | Degree | Location |\n|---|---|---:|---|\n")
	for _, n := range shown {
		label := cell(n.Label)
		if href := opts.linkTo(n.ID); href != "" {
			label = fmt.Sprintf("[%s](%s)", label, href)
		}
		loc := ""
		if n.File != "" {
			loc = fmt.Sprintf("`%s:%d`", cell(n.File), n.Span.StartLine)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s |\n", label, n.Kind, n.Degree, loc)
	}
	if rest := len(members) - len(shown); rest > 0 {
		fmt.Fprintf(&b, "\n… %d more entities — `graphy explain %q`\n", rest, "Community: "+c.Label)
	}
	return b.String()
}

// cell escapes a value for a markdown table cell. Rationale nodes are community
// members and their labels are raw comment prose, so an unescaped pipe silently
// splits the row into extra columns.
func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "|", "\\|")
}

// removeConcept deletes a stale concept file and evicts it from the bundle's
// search index and graph database, via the shared write path so a failure to
// evict is reported rather than swallowed. The index.md refresh is deferred to
// the caller's single syncBundle pass — DeleteConcept would walk the whole
// bundle once per deleted concept.
func removeConcept(cfg *config.Config, rec store.ExportRecord) error {
	abs := filepath.Join(cfg.RootPath, filepath.FromSlash(rec.RelPath))
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return nil // already gone; the record is dropped by the caller
	}
	_, err := bundle.DeleteConceptFile(cfg, abs)
	return err
}

// conceptRef is a concept this export is responsible for: where it lives, and
// the id both the search index and the graph store key on.
type conceptRef struct {
	rel string
	id  string
}

// syncBundle refreshes the bundle's OKF index.md listings after an export.
//
// It used to reconcile two caches — the Bleve search index and the bbolt graph
// database — against every concept this export placed, changed or not. That
// reconciliation existed because the caches could be missing for reasons the
// export could not see: after `.okfw/.index` was lost, a re-export found every
// concept byte-identical, wrote nothing, indexed nothing, and left
// `okfw search --node-id` returning zero hits for good.
//
// Both caches are gone. Search and the link graph are computed from the files,
// so an export that writes correct files has nothing left to keep in step, and
// that entire failure mode cannot recur. What remains is the one derived
// artifact OKF specifies (section 8) and that lives in the bundle as content.
func syncBundle(cfg *config.Config) error {
	_, err := bundle.WriteIndexFiles(cfg, cfg.RootPath)
	return err
}

// qualOfNodeID returns just the qualified-name half of a node id.
func qualOfNodeID(id string) string {
	_, _, _, qual, _ := parseNodeID(id)
	return qual
}
