package bundle

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"okfw/cli/internal/config"
)

// AutoSync refreshes the OKF index.md listings after a concept is created or
// updated — including an index.md for any directory the concept just created.
//
// It used to also write a Bleve document and a bbolt graph node. Both are gone:
// search and the link graph are now computed from the files on demand, so there
// is nothing to keep in step and nothing that can fall out of step. What
// remains is the one artifact OKF actually specifies (section 8) and that a
// consumer expects to find on disk.
func AutoSync(cfg *config.Config, filePath string) error {
	if _, err := WriteIndexFiles(cfg, cfg.RootPath); err != nil {
		return fmt.Errorf("failed to refresh index.md files: %w", err)
	}
	return nil
}

// IndexMarker identifies an index.md that okfw generated and so may overwrite
// or delete. OKF lets index files exist without any tool (section 8), and a
// bundle can hold listings someone wrote by hand; okfw used to treat every
// index.md as its own, so a single `okfw create` replaced a hand-written root
// listing and deleted any index.md in a directory holding no okfw concept.
const IndexMarker = "<!-- okfw:generated — maintained by okfw; edits here are overwritten -->"

// IsGeneratedIndex reports whether index.md content carries IndexMarker.
func IsGeneratedIndex(content []byte) bool {
	return bytes.Contains(content, []byte("<!-- okfw:generated"))
}

// WriteIndexFiles regenerates the index.md listings okfw owns: it creates one
// where a directory has none and rewrites those carrying IndexMarker, leaving
// any other index.md alone. Only marked listings are ever pruned. Returns the
// number of index files written; a listing that is already current is not
// rewritten.
func WriteIndexFiles(cfg *config.Config, root string) (int, error) {
	return writeIndexFiles(root, false)
}

// AdoptIndexFiles is WriteIndexFiles for an explicit regeneration (`okfw
// index-md`, `okfw migrate`): it also takes over unmarked index.md files in
// directories that hold okfw concepts, which is how listings written before the
// marker existed come back under maintenance. It still never prunes one.
func AdoptIndexFiles(cfg *config.Config, root string) (int, error) {
	return writeIndexFiles(root, true)
}

func writeIndexFiles(root string, adopt bool) (int, error) {
	concepts := map[string][]indexEntry{}
	dirs := map[string]bool{"": true}
	// ours marks directories holding a concept okfw wrote (one with an id):
	// the only places an unmarked listing is adopted. A directory of concepts
	// okfw did not write is where a hand-written listing of them lives.
	ours := map[string]bool{}

	addDirChain := func(d string) {
		for {
			dirs[d] = true
			if d == "" {
				break
			}
			d = RelDir(d)
		}
	}

	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".md") || IsReservedBase(fi.Name()) {
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		d := RelDir(rel)
		title := z.Title
		if title == "" {
			title = fi.Name()
		}
		concepts[d] = append(concepts[d], indexEntry{title, z.Description, fi.Name(), z.Type})
		if z.ID != "" {
			ours[d] = true
		}
		addDirChain(d)
		return nil
	})
	if err != nil {
		return 0, err
	}

	// Immediate subdirectories per directory.
	subdirs := map[string][]string{}
	for d := range dirs {
		if d == "" {
			continue
		}
		parent := RelDir(d)
		name := strings.TrimPrefix(d[len(parent):], "/")
		subdirs[parent] = append(subdirs[parent], name)
	}

	count := 0
	for d := range dirs {
		var b strings.Builder
		if d == "" {
			// OKF v0.2 section 12: the bundle-root index.md is the only place
			// frontmatter is permitted in an index file, and okf_version is
			// the only key it may carry.
			b.WriteString("---\nokf_version: \"" + config.OKFVersion + "\"\n---\n\n")
		}
		heading := d
		if heading == "" {
			heading = "/"
		}
		b.WriteString("# " + heading + "\n\n")
		b.WriteString(IndexMarker + "\n\n")
		if subs := subdirs[d]; len(subs) > 0 {
			sort.Strings(subs)
			b.WriteString("## Subdirectories\n\n")
			for _, s := range subs {
				b.WriteString("* [" + escapeLinkText(s) + "/](" + escapeHref(s) + "/)\n")
			}
			b.WriteString("\n")
		}
		if cs := concepts[d]; len(cs) > 0 {
			sort.Slice(cs, func(i, j int) bool { return cs[i].title < cs[j].title })
			b.WriteString("## Concepts\n\n")
			if len(cs) <= indexEnumerateLimit {
				for _, c := range cs {
					b.WriteString(indexLine(c.title, c.desc, c.href))
				}
			} else {
				writeGroupedIndex(&b, cs)
			}
			b.WriteString("\n")
		}
		out := filepath.Join(root, filepath.FromSlash(d), "index.md")
		want := []byte(b.String())
		if have, err := os.ReadFile(out); err == nil {
			if bytes.Equal(have, want) {
				continue
			}
			if !IsGeneratedIndex(have) && !(adopt && ours[d]) {
				continue // someone else's listing
			}
		}
		if err := os.WriteFile(out, want, 0644); err != nil {
			return count, err
		}
		count++
	}

	if err := pruneOrphanedIndexes(root, dirs); err != nil {
		return count, err
	}
	return count, nil
}

// indexEnumerateLimit is the largest number of concepts a directory listing
// will name one by one. Past it the listing groups by type and shows a sample.
//
// OKF section 8 gives index.md one job — "letting a human or agent see what is
// available before opening individual documents" — and a listing that names
// every concept stops doing that job precisely when a directory gets big
// enough for the question to matter. Measured on a generated bundle: 901
// concepts in one directory produced a 112KB index.md, about 28,000 tokens, against a median concept of roughly 1,200. Reading the preview cost
// 23x reading the thing it previewed, and 5x the entire MCP tool-definition
// prefix. That is not disclosure, it is a trap laid by the spec's own
// affordance.
//
// 50 keeps a full listing under roughly 6KB, comfortably cheaper than the
// concepts it stands in for, which is the property that makes it a preview.
const indexEnumerateLimit = 50

// indexEntry is one concept as a directory listing sees it.
type indexEntry struct{ title, desc, href, typ string }

// indexPerTypeLimit is how many concepts of each type a grouped listing names.
// The counts carry the shape of the directory; the sample gives somewhere to
// start reading.
const indexPerTypeLimit = 10

func indexLine(title, desc, href string) string {
	line := "* [" + escapeLinkText(title) + "](" + escapeHref(href) + ")"
	// A listing entry is one line: a newline in a description would end the
	// bullet and spill the rest into the listing as free text.
	if desc = strings.Join(strings.Fields(desc), " "); desc != "" {
		line += " - " + desc
	}
	return line + "\n"
}

// escapeLinkText escapes the characters that would end or nest a markdown link
// text. A title of "Weird ] title [x](evil.md)" otherwise produced a working
// link to evil.md in the listing.
func escapeLinkText(s string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(s)
}

// escapeHref percent-encodes the characters that end or break a markdown link
// destination; okfw's own filenames never contain them, but a hand-named
// concept can.
func escapeHref(s string) string {
	return strings.NewReplacer("%", "%25", " ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(s)
}

// writeGroupedIndex renders a large directory as counts by type plus a sample
// of each, naming the query that returns the rest.
//
// Withheld entries are always counted and always say how to get them, the same
// discipline the MCP tools apply to capped results. A reader who cannot see
// that rows were withheld reads a partial listing as the whole directory.
func writeGroupedIndex(b *strings.Builder, cs []indexEntry) {
	byType := map[string][]indexEntry{}
	for _, c := range cs {
		t := c.typ
		if t == "" {
			t = "untyped"
		}
		byType[t] = append(byType[t], c)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)

	fmt.Fprintf(b, "%d concepts, grouped by type. Listings are capped; "+
		"`okfw search --type <type>` returns a full set.\n\n", len(cs))
	for _, t := range types {
		group := byType[t]
		fmt.Fprintf(b, "### %s (%d)\n\n", t, len(group))
		shown := group
		if len(shown) > indexPerTypeLimit {
			shown = shown[:indexPerTypeLimit]
		}
		for _, c := range shown {
			b.WriteString(indexLine(c.title, c.desc, c.href))
		}
		if n := len(group) - len(shown); n > 0 {
			// Plain text, not a bullet: OKF section 8 listing entries are links.
			fmt.Fprintf(b, "\n… %d more — `okfw search --type %s`\n", n, t)
		}
		b.WriteString("\n")
	}
}

// pruneOrphanedIndexes removes generated index.md files in directories that no
// longer hold any concepts, and the directory itself once it is empty.
//
// Without this, emptying a directory left its listing behind still advertising
// the concepts that used to be there — a generated file making false claims,
// which `okfw delete` and `okfw move` now produce routinely. Only a listing
// carrying IndexMarker is removed: an unmarked index.md is someone else's, and
// a directory with no okfw concept in it is exactly where a hand-written
// listing of foreign concepts lives.
func pruneOrphanedIndexes(root string, keep map[string]bool) error {
	var stale []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if SkipWalkDir(root, p, fi.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.Name() != "index.md" {
			return nil
		}
		rel, rerr := filepath.Rel(root, filepath.Dir(p))
		if rerr != nil {
			return nil
		}
		d := filepath.ToSlash(rel)
		if d == "." {
			d = "" // the bundle root's index.md is always kept
		}
		if keep[d] {
			return nil
		}
		if content, err := os.ReadFile(p); err == nil && IsGeneratedIndex(content) {
			stale = append(stale, p)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, p := range stale {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		// Tidy up the directory when nothing else is left in it. Deliberately
		// not recursive: os.Remove only succeeds on an empty directory, so a
		// parent holding other content is untouched.
		_ = os.Remove(filepath.Dir(p))
	}
	return nil
}
