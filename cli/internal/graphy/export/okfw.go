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
	"path/filepath"
	"sort"
	"strings"

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
}

// OkfwResult summarizes an export.
type OkfwResult struct {
	Created int
	Updated int
	Deleted int
	Dir     string // bundle-relative concept directory
}

// communityKeyPrefix namespaces community entries in the exports bucket so
// they cannot collide with node ids.
const communityKeyPrefix = "\x00community:"

// item is one concept to emit (a community or a node).
type item struct {
	key         string // exports-bucket key
	title       string
	description string
	tags        []string
	node        *model.Node      // nil for communities
	community   *store.Community // nil for nodes
}

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

	items, nodeCommunity, err := selectItems(s, nodes, opts)
	if err != nil {
		return nil, err
	}

	// Pass 1: ensure every item has a concept file; collect key → placement.
	type placement struct {
		conceptID string
		relPath   string
	}
	placed := map[string]placement{}
	var createdPaths []string
	for _, it := range items {
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
		createdPaths = append(createdPaths, rel)
		res.Created++
	}
	created := map[string]bool{}
	for _, p := range createdPaths {
		created[p] = true
	}

	// Pass 2: regenerate every concept's frontmatter fields and body, with
	// links now that all placements are known.
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
		content, err := os.ReadFile(abs)
		if err != nil {
			return nil, err
		}
		updates := map[string]string{
			"title":       it.title,
			"description": it.description,
		}
		if it.node != nil {
			if c, ok := nodeCommunity[it.node.ID]; ok {
				if cp, ok := placed[communityKey(c)]; ok {
					updates["parent"] = cp.conceptID
				}
			}
		}
		content, err = bundle.UpdateFrontmatter(content, updates)
		if err != nil {
			return nil, fmt.Errorf("updating %s: %w", p.relPath, err)
		}
		var body string
		if it.community != nil {
			body = communityBody(it, nodes, linkTo)
		} else {
			body = nodeBody(s, it, nodes, linkTo)
		}
		content, err = spliceBody(content, "# "+it.title+"\n\n"+body)
		if err != nil {
			return nil, fmt.Errorf("splicing %s: %w", p.relPath, err)
		}
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
			title:       "Community: " + c.Label,
			description: fmt.Sprintf("graphy community of %d code entities", len(c.NodeIDs)),
			tags:        []string{"graphy", "graphy/community"},
			community:   &c,
		})
	}

	if !opts.CommunitiesOnly {
		var selected []model.Node
		for _, n := range nodes {
			if n.External || n.Kind == model.KindFile || n.Kind == model.KindHeading {
				continue
			}
			if n.Degree < opts.MinDegree {
				continue
			}
			selected = append(selected, n)
		}
		sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
		for i := range selected {
			n := selected[i]
			desc := n.Doc
			if desc == "" {
				desc = n.Signature
			}
			items = append(items, item{
				key:         n.ID,
				title:       n.Label,
				description: desc,
				tags:        []string{"graphy", "graphy/" + n.Language, "graphy/" + string(n.Kind)},
				node:        &n,
			})
		}
	}
	return items, nodeCommunity, nil
}

func communityKey(id int) string { return fmt.Sprintf("%s%d", communityKeyPrefix, id) }

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

	out, _ := s.OutEdges(n.ID)
	in, _ := s.InEdges(n.ID)
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
				fmt.Fprintf(&b, "- %s [%s](%s) (%s)\n", e.Relation, label, href, e.Confidence)
			} else if ok && other.File != "" {
				fmt.Fprintf(&b, "- %s %s — `%s:%d` (%s)\n", e.Relation, label, other.File, other.Span.StartLine, e.Confidence)
			} else {
				fmt.Fprintf(&b, "- %s %s (%s)\n", e.Relation, label, e.Confidence)
			}
		}
	}
	writeEdges("Outgoing", out, true)
	writeEdges("Incoming", in, false)
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

// spliceBody replaces everything after the frontmatter block with body.
func spliceBody(content []byte, body string) ([]byte, error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("no frontmatter found")
	}
	rest := content[3:]
	end := bytes.Index(rest, []byte("\n---"))
	if end == -1 {
		return nil, fmt.Errorf("frontmatter closing not found")
	}
	head := content[:3+end+4] // include closing ---
	var out bytes.Buffer
	out.Write(head)
	out.WriteString("\n\n")
	out.WriteString(strings.TrimRight(body, "\n"))
	out.WriteString("\n")
	return out.Bytes(), nil
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
