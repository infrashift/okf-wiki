package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graphdb"
	"okfw/cli/internal/index"
)

// Indexer is the minimal indexing surface IndexFile needs (satisfied by
// bleve.Index).
type Indexer interface {
	Index(string, interface{}) error
}

// IndexFile parses a concept file and indexes it as a ConceptDoc.
func IndexFile(idx Indexer, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return err
	}

	body := config.ExtractBody(content)

	docKind := z.GetKind()
	docType := z.Type
	if docType == "" {
		docType = docKind // legacy notes: fall back to the structured kind
	}

	doc := &index.ConceptDoc{
		ID:          z.ID,
		Title:       z.Title,
		Type:        docType,
		Kind:        docKind,
		Description: z.Description,
		Project:     z.Project,
		Tags:        z.Tags,
		Body:        body,
		FilePath:    filePath,
		Created:     z.Created.Format("2006-01-02T15:04:05Z07:00"),
		Timestamp:   z.Timestamp.Format("2006-01-02T15:04:05Z07:00"),
		Status:      z.LifecycleStatus(),
		StaleAfter:  z.StaleAfter,
	}
	if z.Graphy != nil {
		doc.GraphyNodeID = z.Graphy.NodeID
	}

	return idx.Index(z.ID, doc)
}

// AutoIndex opens the bundle's search index and indexes a single file.
func AutoIndex(cfg *config.Config, filePath string) error {
	idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
	if err != nil {
		return err
	}
	defer idx.Close()

	return IndexFile(idx, filePath)
}

// AutoSync updates the search index and, when a graph database exists, the
// persistent graph for a single created or updated file (best-effort).
func AutoSync(cfg *config.Config, filePath string) error {
	if err := AutoIndex(cfg, filePath); err != nil {
		return err
	}

	// Keep OKF index.md listings fresh — including an index.md for any
	// directory the note just created.
	if _, err := WriteIndexFiles(cfg, cfg.RootPath); err != nil {
		return fmt.Errorf("failed to refresh index.md files: %w", err)
	}

	dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
	if _, err := os.Stat(dbPath); err != nil {
		return nil // no graph database — nothing to keep in sync
	}
	store, err := graphdb.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.SyncFile(filePath, cfg.RootPath)
}

// LiveConceptIDs returns the frontmatter ids of every concept currently on disk
// in the bundle.
func LiveConceptIDs(root string) (map[string]bool, error) {
	live := map[string]bool{}
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
		if id := config.ReadConceptID(p); id != "" {
			live[id] = true
		}
		return nil
	})
	return live, err
}

// PruneIndex drops search-index entries whose concept file no longer exists and
// returns how many were removed.
//
// Nothing used to do this: index.DeleteDocument had no callers, so a deleted
// concept stayed in `okfw search` results indefinitely — search reports index
// hits without stat-ing the file, and the only remedy was deleting .index by
// hand.
func PruneIndex(cfg *config.Config) (int, error) {
	if _, err := os.Stat(cfg.IndexPath); err != nil {
		return 0, nil // no search index yet — nothing to prune
	}
	idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
	if err != nil {
		return 0, err
	}
	defer idx.Close()

	indexed, err := index.AllDocIDs(idx)
	if err != nil {
		return 0, err
	}
	live, err := LiveConceptIDs(cfg.RootPath)
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, id := range indexed {
		if live[id] {
			continue
		}
		if err := index.DeleteDocument(idx, id); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// WriteIndexFiles regenerates the auto-maintained index.md listing in every
// bundle directory. Returns the number of index files written.
func WriteIndexFiles(cfg *config.Config, root string) (int, error) {
	type entry struct{ title, desc, href string }
	concepts := map[string][]entry{}
	dirs := map[string]bool{"": true}

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
		if err != nil || z.ID == "" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		d := RelDir(rel)
		title := z.Title
		if title == "" {
			title = fi.Name()
		}
		concepts[d] = append(concepts[d], entry{title, z.Description, fi.Name()})
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
		if subs := subdirs[d]; len(subs) > 0 {
			sort.Strings(subs)
			b.WriteString("## Subdirectories\n\n")
			for _, s := range subs {
				b.WriteString("* [" + s + "/](" + s + "/)\n")
			}
			b.WriteString("\n")
		}
		if cs := concepts[d]; len(cs) > 0 {
			sort.Slice(cs, func(i, j int) bool { return cs[i].title < cs[j].title })
			b.WriteString("## Concepts\n\n")
			for _, c := range cs {
				line := "* [" + c.title + "](" + c.href + ")"
				if c.desc != "" {
					line += " - " + c.desc
				}
				b.WriteString(line + "\n")
			}
			b.WriteString("\n")
		}
		out := filepath.Join(root, filepath.FromSlash(d), "index.md")
		if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
			return count, err
		}
		count++
	}

	if err := pruneOrphanedIndexes(root, dirs); err != nil {
		return count, err
	}
	return count, nil
}

// pruneOrphanedIndexes removes generated index.md files in directories that no
// longer hold any concepts, and the directory itself once it is empty.
//
// Without this, emptying a directory left its listing behind still advertising
// the concepts that used to be there — a generated file making false claims,
// which `okfw delete` and `okfw move` now produce routinely. index.md is
// okfw-owned and overwritten on every change, so removing one is in remit.
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
		if !keep[d] {
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
