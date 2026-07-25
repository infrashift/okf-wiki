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
		Status:      z.Status,
		Due:         z.Due,
		Completed:   z.Completed,
		Priority:    z.Priority,
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
			b.WriteString("---\nokf_version: \"0.1\"\n---\n\n")
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
	return count, nil
}
