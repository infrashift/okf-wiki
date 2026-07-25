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
}

// OkfwResult summarizes an export.
type OkfwResult struct {
	Created   int
	Updated   int
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
	resource    string // OKF resource: source location (or permalink)
	language    string
	entity      string // node kind or "community"
	tags        []string
	node        *model.Node      // nil for communities
	community   *store.Community // nil for nodes
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
	items, nodeCommunity, err := selectItems(s, nodes, opts, commit)
	if err != nil {
		return nil, err
	}

	// Pass 1: ensure every item has a concept file; collect key → placement.
	// When the exports mapping is missing (e.g. .graphy/ was deleted), adopt
	// existing concepts by their embedded node identity instead of
	// duplicating them.
	placed := map[string]placement{}
	var recovered map[string]placement // rid → placement; scanned lazily
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

	// Pass 2: rewrite every concept in full — frontmatter (preserving id and
	// created) and body — with links now that all placements are known.
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
		if it.node != nil {
			if c, ok := nodeCommunity[it.node.ID]; ok {
				if cp, ok := placed[communityKey(c)]; ok {
					parentID = cp.conceptID
				}
			}
		}
		var body string
		if it.community != nil {
			body = communityBody(it, nodes, linkTo)
		} else {
			body = nodeBody(s, it, nodes, linkTo)
		}
		content := renderConcept(prev, it, opts, parentID, commit, body)
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
func selectItems(s *store.Store, nodes map[string]model.Node, opts OkfwOptions, commit string) ([]item, map[string]int, error) {
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
				rid:         n.ID,
				title:       n.Label,
				conceptType: "code-" + string(n.Kind),
				description: desc,
				resource:    resourceFor(n, opts.RepoURL, commit),
				language:    n.Language,
				entity:      string(n.Kind),
				tags:        []string{"graphy", "graphy/" + n.Language, "graphy/" + string(n.Kind)},
				node:        &n,
			})
		}
	}
	return items, nodeCommunity, nil
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
// body and the machine-readable node marker.
func renderConcept(prev *config.Concept, it item, opts OkfwOptions, parentID, commit, body string) []byte {
	created := prev.Created
	if created.IsZero() {
		created = time.Now()
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %q\n", prev.ID)
	fmt.Fprintf(&b, "title: %q\n", it.title)
	fmt.Fprintf(&b, "type: %q\n", it.conceptType)
	if it.description != "" {
		fmt.Fprintf(&b, "description: %q\n", it.description)
	}
	if it.resource != "" {
		fmt.Fprintf(&b, "resource: %q\n", it.resource)
	}
	if opts.Project != "" {
		fmt.Fprintf(&b, "project: %q\n", opts.Project)
	}
	b.WriteString("tags:\n")
	for _, tag := range it.tags {
		fmt.Fprintf(&b, "  - %q\n", tag)
	}
	fmt.Fprintf(&b, "created: %q\n", created.Format(time.RFC3339))
	fmt.Fprintf(&b, "timestamp: %q\n", time.Now().Format(time.RFC3339))
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
