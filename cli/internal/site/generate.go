package site

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/cli/internal/bundle"
	"github.com/infrashift/okf-wiki/internal/config"
	"github.com/infrashift/okf-wiki/internal/graph"
)

// sidebarConceptLimit is the bundle size up to which the sidebar names every
// concept. Starlight renders the sidebar into every page, so naming N
// concepts on each of N pages is quadratic: past this, the sidebar lists
// directories, and concepts are reached through the directory pages and
// search.
const sidebarConceptLimit = 300

// assetsPrefix is where a bundle's non-markdown files (OKF assets, section
// 3.2) are published, clear of Astro's own _astro/ and Pagefind's pagefind/.
const assetsPrefix = "assets"

// Stats reports what a generation wrote.
type Stats struct {
	Concepts    int
	Directories int
	Assets      int
	Dangling    int // body links to a concept that does not exist
}

type page struct {
	rel   string // bundle-relative path, with .md
	dir   string
	slug  string
	z     *config.Concept
	body  string
	links []string // bundle-relative paths of linked concepts
}

// Generate writes the Astro content for the bundle at bundleRoot into the
// workdir: one page per concept, one per directory, the bundle's assets, and
// okfw-site.json. It reads the bundle the way every okfw command does — the
// same walk, the same concept rule (OKF v0.3 section 3.2), the same link
// resolution — so the site shows the graph okfw search and okfw graph see.
func Generate(w *Workdir) (*Stats, error) {
	root := w.BundleRoot
	meta, err := bundle.ReadBundleMeta(root)
	if err != nil {
		return nil, err
	}
	exists := bundle.RefExists(root)

	concepts := map[string]*page{} // by rel
	dirs := map[string]bool{"": true}
	indexBodies := map[string]string{} // dir -> body of its index.md
	var assets []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && bundle.SkipWalkDir(root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 && !bundle.Within(root, p) {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(path.Base(rel), ".") {
			return nil
		}
		if !strings.HasSuffix(rel, ".md") {
			assets = append(assets, rel)
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		dir := bundle.RelDir(rel)
		if path.Base(rel) == "index.md" {
			indexBodies[dir] = config.ExtractBody(content)
			return nil
		}
		if config.IsReservedBase(path.Base(rel)) {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil || !config.IsConcept(z) {
			return nil
		}
		concepts[rel] = &page{rel: rel, dir: dir, z: z, body: config.ExtractBody(content)}
		for d := dir; ; d = bundle.RelDir(d) {
			dirs[d] = true
			if d == "" {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Slugs: lowercase and URL-safe, unique, and set explicitly in each page's
	// frontmatter so Astro's own slugifier never decides a URL.
	used := map[string]bool{"": true}
	slugOf := func(want string) string {
		s := want
		for i := 2; used[s]; i++ {
			s = fmt.Sprintf("%s-%d", want, i)
		}
		used[s] = true
		return s
	}
	conceptRels := sortedKeys(concepts)
	for _, rel := range conceptRels {
		concepts[rel].slug = slugOf(slugify(strings.TrimSuffix(rel, ".md")))
	}
	dirSlug := map[string]string{"": ""}
	for _, d := range sortedKeys(dirs) {
		if d != "" {
			dirSlug[d] = slugOf(slugify(d))
		}
	}
	href := func(slug string) string {
		if slug == "" {
			return "/"
		}
		return "/" + slug + "/"
	}
	assetSet := map[string]bool{}
	for _, a := range assets {
		assetSet[a] = true
	}

	// Links, resolved as okfw resolves them, and backlinks from them.
	backlinks := map[string][]string{}
	stats := &Stats{Concepts: len(concepts), Directories: len(dirs), Assets: len(assets)}
	keyOf := func(rel string) string {
		if _, ok := concepts[rel]; ok {
			return rel
		}
		return ""
	}
	for _, rel := range conceptRels {
		pg := concepts[rel]
		pg.links = config.ResolveConceptLinks(pg.z, pg.body, pg.dir, keyOf, exists)
		for _, to := range pg.links {
			if to != rel {
				backlinks[to] = append(backlinks[to], rel)
			}
		}
	}

	// target turns a link destination, as written in a file at fromDir, into
	// the site URL it should open, or "" to leave it as written.
	target := func(dest, fromDir string) string {
		if dest == "" || strings.HasPrefix(dest, "#") || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
			return ""
		}
		rel := graph.NormalizeTarget(dest, fromDir)
		switch {
		case rel == "" && (dest == "/" || dest == "."):
			return href(dirSlug[fromDir])
		case rel == "":
			return ""
		}
		if pg, ok := concepts[rel]; ok {
			return href(pg.slug)
		}
		if path.Base(rel) == "index.md" {
			rel = bundle.RelDir(rel)
		}
		if s, ok := dirSlug[rel]; ok {
			return href(s)
		}
		if assetSet[rel] {
			return "/" + assetsPrefix + "/" + rel
		}
		return ""
	}

	content := filepath.Join(w.Dir, "src", "content", "docs")
	public := filepath.Join(w.Dir, "public", assetsPrefix)
	for _, d := range []string{content, public} {
		if err := os.RemoveAll(d); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}

	now := time.Now()
	for _, rel := range conceptRels {
		pg := concepts[rel]
		z := pg.z
		body, dangling := rewriteBody(pg.body, pg.dir, target)
		stats.Dangling += dangling

		o := okfData{
			Path:   rel,
			Type:   z.Type,
			Tags:   []string(z.Tags),
			Status: z.LifecycleStatus(),
			Stale:  z.IsStale(now),
			Trust:  z.TrustTier(),
		}
		if z.StaleAfter != "" {
			o.StaleAfter = z.StaleAfter
		}
		if z.Generated != nil {
			o.GeneratedBy, o.GeneratedAt = z.Generated.By, z.Generated.At
		}
		current := map[config.Verification]bool{}
		for _, v := range z.CurrentVerifications() {
			current[v] = true
		}
		for _, v := range z.Verified {
			if v.By != "" && v.At != "" {
				o.Verified = append(o.Verified, verifiedData{By: v.By, At: v.At, Current: current[v]})
			}
		}
		for _, r := range z.PathRefs() {
			pr := pathRefData{Field: r.Field, Value: r.Value}
			if t := graph.ResolveRef(r.Value, pg.dir, exists); t != "" {
				if c, ok := concepts[t]; ok {
					pr.Href = href(c.slug)
					if r.Field == "superseded_by" {
						o.SupersededBy = &linkData{Title: titleOf(c), Href: pr.Href}
					}
				}
			} else if h := target(r.Value, pg.dir); h != "" {
				pr.Href = h
			}
			o.PathRefs = append(o.PathRefs, pr)
		}
		for _, from := range backlinks[rel] {
			c := concepts[from]
			o.Backlinks = append(o.Backlinks, linkData{Title: titleOf(c), Href: href(c.slug)})
		}
		fm := frontmatter{
			Title:       titleOf(pg),
			Description: oneLine(z.Description),
			Slug:        pg.slug,
			OKF:         &o,
		}
		if err := writePage(content, pg.slug, fm, body); err != nil {
			return nil, err
		}
	}

	// Directory pages: the bundle's own index.md listing where there is one
	// (okfw writes one per directory), otherwise one generated here.
	for _, d := range sortedKeys(dirs) {
		listing, ok := indexBodies[d]
		if !ok {
			listing = generatedListing(d, dirs, concepts)
		}
		listing = stripLeadingH1(listing)
		body, _ := rewriteBody(listing, d, target)
		fm := frontmatter{Title: d + "/", Slug: dirSlug[d]}
		if d == "" {
			fm.Title = meta.Title
			if fm.Title == "" {
				fm.Title = filepath.Base(filepath.Dir(root))
			}
			fm.Description = oneLine(meta.Description)
			fm.Slug = ""
		}
		if err := writePage(content, dirSlug[d], fm, body); err != nil {
			return nil, err
		}
	}

	for _, a := range assets {
		src := filepath.Join(root, filepath.FromSlash(a))
		dst := filepath.Join(public, filepath.FromSlash(a))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, err
		}
	}

	title := meta.Title
	if title == "" {
		title = filepath.Base(filepath.Dir(root))
	}
	cfg := siteConfig{
		Title:       title,
		Description: oneLine(meta.Description),
		Sidebar:     sidebar(concepts, conceptRels, dirs, dirSlug, href),
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(w.Dir, "okfw-site.json"), data, 0o644); err != nil {
		return nil, err
	}
	return stats, nil
}

type frontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description,omitempty"`
	Slug        string   `yaml:"slug,omitempty"`
	OKF         *okfData `yaml:"okf,omitempty"`
}

type okfData struct {
	Path         string         `yaml:"path"`
	Type         string         `yaml:"type,omitempty"`
	Tags         []string       `yaml:"tags"`
	Status       string         `yaml:"status"`
	Stale        bool           `yaml:"stale"`
	StaleAfter   string         `yaml:"staleAfter,omitempty"`
	Trust        string         `yaml:"trust"`
	GeneratedBy  string         `yaml:"generatedBy,omitempty"`
	GeneratedAt  string         `yaml:"generatedAt,omitempty"`
	Verified     []verifiedData `yaml:"verified"`
	SupersededBy *linkData      `yaml:"supersededBy,omitempty"`
	PathRefs     []pathRefData  `yaml:"pathRefs"`
	Backlinks    []linkData     `yaml:"backlinks"`
}

type verifiedData struct {
	By      string `yaml:"by"`
	At      string `yaml:"at"`
	Current bool   `yaml:"current"`
}

type linkData struct {
	Title string `yaml:"title" json:"label,omitempty"`
	Href  string `yaml:"href" json:"link,omitempty"`
}

type pathRefData struct {
	Field string `yaml:"field"`
	Value string `yaml:"value"`
	Href  string `yaml:"href,omitempty"`
}

type siteConfig struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Sidebar     []sidebarItem `json:"sidebar"`
}

// sidebarItem is a Starlight sidebar entry: a link, a group of items, or a
// group autogenerated from a content directory.
type sidebarItem struct {
	Label        string        `json:"label"`
	Link         string        `json:"link,omitempty"`
	Items        []sidebarItem `json:"items,omitempty"`
	Collapsed    bool          `json:"collapsed,omitempty"`
	Autogenerate *autogen      `json:"autogenerate,omitempty"`
}

type autogen struct {
	Directory string `json:"directory"`
}

// writePage writes one content page. Its frontmatter goes through yaml.v3, so
// a title holding a colon, a quote or a "---" cannot break out of it; the body
// is the concept's markdown with only its link destinations changed.
func writePage(content, slug string, fm frontmatter, body string) error {
	file := "index.md"
	if slug != "" {
		file = slug + ".md"
	}
	// A directory page and a concept can share a parent path; the page file
	// name never decides the URL (slug does), so keep files apart.
	if fm.OKF == nil && slug != "" {
		file = slug + "/index.md"
	}
	out := filepath.Join(content, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(fm)
	if err != nil {
		return err
	}
	return os.WriteFile(out, []byte("---\n"+string(data)+"---\n\n"+body), 0o644)
}

func titleOf(pg *page) string {
	if t := strings.TrimSpace(pg.z.Title); t != "" {
		return oneLine(t)
	}
	return strings.TrimSuffix(path.Base(pg.rel), ".md")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

var slugUnsafe = regexp.MustCompile(`[^a-z0-9._-]+`)

// slugify maps a bundle path to a URL path: lowercase, each segment reduced
// to [a-z0-9._-], so the URL means the same in every browser and host.
func slugify(p string) string {
	segs := strings.Split(strings.ToLower(p), "/")
	for i, s := range segs {
		s = strings.Trim(slugUnsafe.ReplaceAllString(s, "-"), "-.")
		if s == "" {
			s = "-"
		}
		segs[i] = s
	}
	return strings.Join(segs, "/")
}

// linkDestRe finds inline link and image destinations: ](dest) and
// ](dest "title"). Reference definitions are left to graph.RewriteLinks.
var linkDestRe = regexp.MustCompile(`\]\(\s*(<[^>]*>|[^)\s]+)`)

// rewriteBody points every link that names something in the bundle at its
// page on the site: concepts and index.md listings through
// graph.RewriteLinks, the parser okfw's other commands use, and directories
// and assets with a second pass that skips fenced code. It reports how many
// links to a missing concept it left as written.
func rewriteBody(body, fromDir string, target func(dest, fromDir string) string) (string, int) {
	dangling := 0
	// The first pass marks what it rewrote, so the second never reads a site
	// URL it produced as a bundle path: /tables/ is a concept's page here, and
	// may also be the name of a bundle directory.
	body = graph.RewriteLinks(body, func(h string) string {
		if t := target(h, fromDir); t != "" {
			return rewrittenMark + t
		}
		dangling++
		return ""
	})
	var b strings.Builder
	inFence := false
	for _, line := range strings.SplitAfter(body, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
		}
		if inFence || !strings.Contains(line, "](") {
			b.WriteString(line)
			continue
		}
		b.WriteString(linkDestRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := linkDestRe.FindStringSubmatch(m)
			dest := strings.Trim(sub[1], "<>")
			if strings.HasPrefix(dest, rewrittenMark) || strings.HasSuffix(strings.ToLower(strings.SplitN(dest, "#", 2)[0]), ".md") {
				return m // the first pass's, or a .md link it left as written
			}
			frag := ""
			if i := strings.IndexByte(dest, '#'); i >= 0 {
				dest, frag = dest[:i], dest[i:]
			}
			if t := target(dest, fromDir); t != "" {
				return "](" + t + frag
			}
			return m
		}))
	}
	return strings.ReplaceAll(b.String(), rewrittenMark, ""), dangling
}

// rewrittenMark prefixes a destination the first rewrite pass produced. It
// cannot occur in a markdown file by accident.
const rewrittenMark = "okfw-site-rewritten:"

// stripLeadingH1 drops a listing's first "# heading": Starlight renders the
// page title itself.
func stripLeadingH1(body string) string {
	lines := strings.SplitN(strings.TrimLeft(body, "\n"), "\n", 2)
	if len(lines) > 0 && strings.HasPrefix(lines[0], "# ") {
		if len(lines) == 2 {
			return lines[1]
		}
		return ""
	}
	return body
}

// generatedListing writes a directory listing for a directory with no
// index.md: subdirectories, then concepts with their descriptions.
func generatedListing(dir string, dirs map[string]bool, concepts map[string]*page) string {
	var b strings.Builder
	var subs []string
	for d := range dirs {
		if d != "" && d != dir && bundle.RelDir(d) == dir {
			subs = append(subs, d)
		}
	}
	sort.Strings(subs)
	if len(subs) > 0 {
		b.WriteString("## Subdirectories\n\n")
		for _, d := range subs {
			name := path.Base(d)
			fmt.Fprintf(&b, "* [%s/](%s/)\n", name, name)
		}
		b.WriteString("\n")
	}
	var here []*page
	for _, pg := range concepts {
		if pg.dir == dir {
			here = append(here, pg)
		}
	}
	sort.Slice(here, func(i, j int) bool { return titleOf(here[i]) < titleOf(here[j]) })
	if len(here) > 0 {
		b.WriteString("## Concepts\n\n")
		for _, pg := range here {
			line := fmt.Sprintf("* [%s](%s)", strings.NewReplacer("[", `\[`, "]", `\]`).Replace(titleOf(pg)), path.Base(pg.rel))
			if d := oneLine(pg.z.Description); d != "" {
				line += " - " + d
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

// sidebar names every concept in a small bundle, by directory; in a large
// one it lists the directories only (see sidebarConceptLimit).
func sidebar(concepts map[string]*page, rels []string, dirs map[string]bool, dirSlug map[string]string, href func(string) string) []sidebarItem {
	items := []sidebarItem{{Label: "Overview", Link: "/"}}
	small := len(concepts) <= sidebarConceptLimit
	if small {
		for _, rel := range rels {
			if pg := concepts[rel]; pg.dir == "" {
				items = append(items, sidebarItem{Label: titleOf(pg), Link: href(pg.slug)})
			}
		}
	}
	var dirItem func(d string) sidebarItem
	dirItem = func(d string) sidebarItem {
		it := sidebarItem{Label: path.Base(d) + "/", Collapsed: true}
		it.Items = append(it.Items, sidebarItem{Label: "Overview", Link: href(dirSlug[d])})
		for _, sub := range sortedKeys(dirs) {
			if sub != "" && sub != d && bundle.RelDir(sub) == d {
				it.Items = append(it.Items, dirItem(sub))
			}
		}
		if small {
			for _, rel := range rels {
				if pg := concepts[rel]; pg.dir == d {
					it.Items = append(it.Items, sidebarItem{Label: titleOf(pg), Link: href(pg.slug)})
				}
			}
		}
		if len(it.Items) == 1 {
			return sidebarItem{Label: it.Label, Link: href(dirSlug[d])}
		}
		return it
	}
	for _, d := range sortedKeys(dirs) {
		if d != "" && bundle.RelDir(d) == "" {
			items = append(items, dirItem(d))
		}
	}
	return items
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
