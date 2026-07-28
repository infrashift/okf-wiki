package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graph"
)

// LoadGraph builds the bundle's link graph by reading the concepts on disk.
//
// There is no persisted graph store. The graph is a pure function of the files
// and rebuilding it costs roughly 50ms on a 900-concept bundle — less than the
// machinery a cache needs to stay honest. The bbolt store this replaces was a
// second source of truth that could disagree with the files, and it is where
// `graph-sync`, prune-on-delete, sync-after-bulk-write and
// open-existing-versus-create all came from: rules about the cache, not about
// the knowledge.
//
// The cost is linear in bundle size, so this is the first thing to revisit if
// bundles ever reach tens of thousands of concepts.
func LoadGraph(bundleRoot string) (*graph.Graph, error) {
	return LoadGraphFrom(bundleRoot, bundleRoot)
}

// LoadGraphFrom builds the graph from a file or a subtree, resolving links
// against bundleRoot. A file argument yields a one-node graph, which is what a
// caller inspecting a single concept wants.
func LoadGraphFrom(path, bundleRoot string) (*graph.Graph, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path %q: %w", path, err)
	}

	var files []string
	if info.IsDir() {
		err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() && SkipWalkDir(path, p, fi.Name()) {
				return filepath.SkipDir
			}
			if !fi.IsDir() && strings.HasSuffix(p, ".md") && !IsReservedBase(fi.Name()) {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk directory: %w", err)
		}
	} else {
		files = []string{path}
	}
	// Pin the order: the walk is sorted per directory, but tie-breaking
	// downstream must not vary between runs.
	sort.Strings(files)

	g := graph.New()
	for _, filePath := range files {
		node, err := parseConceptNode(filePath, bundleRoot)
		if err != nil {
			continue // reserved file, malformed frontmatter, or no id
		}
		g.AddNode(node)
	}
	if g.NodeCount() == 0 {
		return nil, fmt.Errorf("no valid concepts found in %s", path)
	}
	g.BuildRelationships()
	return g, nil
}

func parseConceptNode(filePath, bundleRoot string) (*graph.Node, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}
	if z.ID == "" {
		return nil, fmt.Errorf("not a valid concept: no id field")
	}
	absPath, _ := filepath.Abs(filePath)
	body := config.ExtractBody(content)
	return &graph.Node{
		ID:       z.ID,
		Title:    z.Title,
		FilePath: absPath,
		Project:  z.Project,
		Parent:   z.Parent,
		Tags:     z.Tags,
		Links:    config.ResolveBodyLinkIDs(body, absPath, bundleRoot),
	}, nil
}
