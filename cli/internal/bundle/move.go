package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graph"
	"okfw/cli/internal/graphdb"
	"okfw/cli/internal/index"
)

// Relocating and removing concepts. Both operations have to touch four things
// that can otherwise drift apart: the file, the search index, the graph
// database, and the index.md listings.

// RelHref returns the relative slash path that reaches target from fromDir, both
// bundle-relative. Used to keep a relative markdown link pointing at the same
// concept after one end of it moves.
func RelHref(fromDir, target string) string {
	if fromDir == "" {
		return target
	}
	from := strings.Split(fromDir, "/")
	to := strings.Split(target, "/")

	// Compare directory components only: the last element of `to` is a filename.
	i := 0
	for i < len(from) && i < len(to)-1 && from[i] == to[i] {
		i++
	}
	parts := make([]string, 0, len(from)-i+len(to)-i)
	for j := i; j < len(from); j++ {
		parts = append(parts, "..")
	}
	parts = append(parts, to[i:]...)
	return strings.Join(parts, "/")
}

// retarget rebuilds an href so it still resolves to target from fromDir, keeping
// the original's bundle-absolute or relative form. Preserving the form matters:
// OKF recommends the bundle-absolute style precisely because it survives moves,
// and silently converting a user's relative links would overwrite that choice.
func retarget(href, fromDir, target string) string {
	if strings.HasPrefix(href, "/") {
		return "/" + target
	}
	rel := RelHref(fromDir, target)
	// A bare "x.md" and a "./x.md" mean the same thing; echo back whichever the
	// author used, so the diff stays limited to links that actually moved.
	if strings.HasPrefix(href, "./") && !strings.HasPrefix(rel, "../") {
		rel = "./" + rel
	}
	return rel
}

// relSlash converts an absolute path inside the bundle to a slash-separated
// bundle-relative path.
func relSlash(root, abs string) (string, error) {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s is outside the bundle", abs)
	}
	return rel, nil
}

// eachConcept calls fn for every concept file in the bundle, skipping reserved
// filenames and hidden directories.
func eachConcept(root string, fn func(abs, rel string) error) error {
	return filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
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
		rel, rerr := relSlash(root, p)
		if rerr != nil {
			return nil
		}
		return fn(p, rel)
	})
}

// InboundLinks returns the bundle-relative paths of concepts linking to target
// (itself bundle-relative), excluding target's own self-links.
func InboundLinks(root, target string) ([]string, error) {
	var sources []string
	err := eachConcept(root, func(abs, rel string) error {
		if rel == target {
			return nil
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return nil
		}
		fromDir := RelDir(rel)
		for _, href := range graph.ExtractLinkTargets(config.ExtractBody(content)) {
			if graph.NormalizeTarget(href, fromDir) == target {
				sources = append(sources, rel)
				return nil
			}
		}
		return nil
	})
	return sources, err
}

// DeleteResult reports what a delete touched.
type DeleteResult struct {
	ID      string
	Rel     string
	Inbound []string // concepts left pointing at nothing
}

// DeleteConcept removes a concept and everything derived from it: its search
// index entry, its graph node, and its line in the index.md listings.
//
// Inbound links are reported, not repaired. OKF section 6.1 requires consumers to
// tolerate a link whose target does not exist — it "may simply represent
// not-yet-written knowledge" — so rewriting other concepts' prose would destroy
// authored content to fix something the spec says is not broken.
func DeleteConcept(cfg *config.Config, absPath string) (*DeleteResult, error) {
	rel, err := relSlash(cfg.RootPath, absPath)
	if err != nil {
		return nil, err
	}
	if IsReservedBase(filepath.Base(absPath)) {
		return nil, fmt.Errorf("%s is a reserved generated file, not a concept", rel)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}
	res := &DeleteResult{Rel: rel}
	if z, perr := config.ParseFrontmatter(content); perr == nil {
		res.ID = z.ID
	}

	// Collected before the file goes, since the walk keys off its path.
	res.Inbound, err = InboundLinks(cfg.RootPath, rel)
	if err != nil {
		return nil, err
	}

	if err := os.Remove(absPath); err != nil {
		return nil, err
	}

	if res.ID != "" {
		if err := dropFromIndex(cfg, res.ID); err != nil {
			return nil, err
		}
		if err := purgeFromGraph(cfg, res.ID); err != nil {
			return nil, err
		}
	}
	if _, err := WriteIndexFiles(cfg, cfg.RootPath); err != nil {
		return nil, fmt.Errorf("failed to refresh index.md files: %w", err)
	}
	return res, nil
}

// dropFromIndex removes a concept from the search index, if one exists.
func dropFromIndex(cfg *config.Config, id string) error {
	if _, err := os.Stat(cfg.IndexPath); err != nil {
		return nil
	}
	idx, err := index.OpenOrCreateIndex(cfg.IndexPath)
	if err != nil {
		return err
	}
	defer idx.Close()
	return index.DeleteDocument(idx, id)
}

// purgeFromGraph removes a concept's node from the graph database, if one exists.
func purgeFromGraph(cfg *config.Config, id string) error {
	dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	store, err := graphdb.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.PurgeNode(id)
}

// MoveResult reports what a move touched.
type MoveResult struct {
	FromRel  string
	ToRel    string
	Rewrote  []string // other concepts whose links were retargeted
	SelfLink bool     // the concept linked to itself
}

// MoveConcept relocates a concept within the bundle, keeping its `id` and
// repairing the links at both ends.
//
// Two sets of links break on a move, and missing either corrupts the graph
// silently:
//
//   - Inbound links in other concepts still name the old path.
//   - The moved concept's own *relative* links resolve against its directory, so
//     "./sibling.md" points somewhere else once the file sits elsewhere.
//     Bundle-absolute links are immune, which is why OKF recommends them.
//
// The moved concept's `timestamp` is bumped, because a concept's OKF identity is
// its path — moving it is a meaningful change. The concepts whose links were
// retargeted are not stamped: their knowledge did not change, only the spelling
// of a pointer to the same concept.
func MoveConcept(cfg *config.Config, oldAbs, newAbs string) (*MoveResult, error) {
	oldRel, err := relSlash(cfg.RootPath, oldAbs)
	if err != nil {
		return nil, err
	}
	newRel, err := relSlash(cfg.RootPath, newAbs)
	if err != nil {
		return nil, err
	}
	if IsReservedBase(filepath.Base(newAbs)) {
		return nil, fmt.Errorf("%s is a reserved OKF filename", newRel)
	}
	if !strings.HasSuffix(newRel, ".md") {
		return nil, fmt.Errorf("%s: a concept filename must end in .md", newRel)
	}
	if oldRel == newRel {
		return nil, fmt.Errorf("%s is already there", oldRel)
	}
	if _, err := os.Stat(newAbs); err == nil {
		return nil, fmt.Errorf("%s already exists", newRel)
	}

	content, err := os.ReadFile(oldAbs)
	if err != nil {
		return nil, err
	}
	res := &MoveResult{FromRel: oldRel, ToRel: newRel}
	oldDir, newDir := RelDir(oldRel), RelDir(newRel)

	// 1. Keep the moved concept's own relative links resolving to the same
	//    targets from its new directory.
	body := config.ExtractBody(content)
	fixedBody := graph.RewriteLinks(body, func(href string) string {
		if strings.HasPrefix(href, "/") {
			return "" // bundle-absolute links are unaffected by a move
		}
		target := graph.NormalizeTarget(href, oldDir)
		if target == "" {
			return ""
		}
		if target == oldRel {
			res.SelfLink = true
			return retarget(href, newDir, newRel)
		}
		return retarget(href, newDir, target)
	})

	moved := content
	if fixedBody != body {
		moved, err = ReplaceBody(content, fixedBody)
		if err != nil {
			return nil, err
		}
	}
	// A move changes the concept's OKF identity, so it is a meaningful change.
	moved, err = UpdateFrontmatter(moved, nil)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(newAbs), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(newAbs, moved, 0644); err != nil {
		return nil, err
	}
	if err := os.Remove(oldAbs); err != nil {
		return nil, err
	}

	// 2. Retarget inbound links in every other concept.
	err = eachConcept(cfg.RootPath, func(abs, rel string) error {
		if rel == newRel {
			return nil
		}
		srcContent, rerr := os.ReadFile(abs)
		if rerr != nil {
			return nil
		}
		srcBody := config.ExtractBody(srcContent)
		fromDir := RelDir(rel)
		out := graph.RewriteLinks(srcBody, func(href string) string {
			if graph.NormalizeTarget(href, fromDir) != oldRel {
				return ""
			}
			return retarget(href, fromDir, newRel)
		})
		if out == srcBody {
			return nil
		}
		rewritten, ferr := ReplaceBody(srcContent, out)
		if ferr != nil {
			return nil // not a concept we can safely rewrite
		}
		if werr := os.WriteFile(abs, rewritten, 0644); werr != nil {
			return werr
		}
		res.Rewrote = append(res.Rewrote, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 3. Re-sync everything whose content or location changed. The moved concept
	//    keeps its id, so re-indexing overwrites the stale path in place.
	if err := AutoSync(cfg, newAbs); err != nil {
		return nil, err
	}
	if err := syncGraphRemoval(cfg, oldAbs); err != nil {
		return nil, err
	}
	for _, rel := range res.Rewrote {
		if err := AutoSync(cfg, filepath.Join(cfg.RootPath, filepath.FromSlash(rel))); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// syncGraphRemoval clears any graph node still recorded at a path that no longer
// exists. Harmless when the node was already re-keyed by AutoSync.
func syncGraphRemoval(cfg *config.Config, absPath string) error {
	dbPath := filepath.Join(cfg.RootPath, cfg.GraphPath)
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	store, err := graphdb.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.SyncFile(absPath, cfg.RootPath)
}

// ReplaceBody swaps a concept's markdown body, leaving every byte of the
// frontmatter block — including its line endings — exactly as it was.
//
// The body offset is computed rather than rebuilt from the parsed pieces:
// reassembling "---\n" + yaml + "---\n" would normalise a CRLF file's delimiters
// to LF, rewriting lines the caller never asked to touch.
func ReplaceBody(content []byte, newBody string) ([]byte, error) {
	_, _, rest, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}
	// rest starts at the closing delimiter's line ending; the body follows it,
	// matching where config.ExtractBody considers the body to begin.
	bodyStart := len(content) - len(rest)
	switch {
	case strings.HasPrefix(string(rest), "\r\n"):
		bodyStart += 2
	case strings.HasPrefix(string(rest), "\n"):
		bodyStart++
	}
	out := make([]byte, 0, bodyStart+len(newBody))
	out = append(out, content[:bodyStart]...)
	out = append(out, newBody...)
	return out, nil
}
