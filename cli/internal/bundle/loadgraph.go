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

	type parsed struct {
		node    *graph.Node
		body    string
		fromDir string
	}
	var concepts []parsed
	keyByRel := map[string]string{}
	for _, filePath := range files {
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		z, err := config.ParseFrontmatter(content)
		if err != nil {
			continue // no parseable frontmatter: not a concept
		}
		absPath, _ := filepath.Abs(filePath)
		rel, err := relSlash(bundleRoot, absPath)
		if err != nil {
			continue
		}
		key := config.ConceptKey(z, rel)
		keyByRel[rel] = key
		concepts = append(concepts, parsed{
			node: &graph.Node{
				ID:       key,
				Title:    z.Title,
				FilePath: absPath,
				Project:  z.Project,
				Parent:   z.Parent,
				Tags:     z.Tags,
			},
			body:    config.ExtractBody(content),
			fromDir: RelDir(rel),
		})
	}

	// Resolve links from what the walk already read; only a target outside a
	// subtree walk needs reading.
	keyOf := func(rel string) string {
		if k, ok := keyByRel[rel]; ok {
			return k
		}
		k := config.ReadConceptKey(bundleRoot, rel)
		keyByRel[rel] = k
		return k
	}
	g := graph.New()
	for _, c := range concepts {
		c.node.Links = config.ResolveBodyLinks(c.body, c.fromDir, keyOf)
		g.AddNode(c.node)
	}
	if g.NodeCount() == 0 {
		return nil, fmt.Errorf("no concepts found in %s", path)
	}
	g.BuildRelationships()
	return g, nil
}

// ConceptKeyOf resolves any concept reference a command accepts — frontmatter
// id, OKF concept ID, or file path — to the concept key the link graph uses. A
// bare id that resolves to no file is returned as given, so a query about a
// deleted concept still finds the links left pointing at it.
func ConceptKeyOf(cfg *config.Config, ref string) (string, error) {
	p, err := ResolveConceptPath(cfg, ref)
	if err != nil {
		if strings.HasSuffix(ref, ".md") || strings.Contains(ref, "/") {
			return "", err
		}
		return ref, nil
	}
	rel, err := relSlash(cfg.RootPath, p)
	if err != nil {
		return "", err
	}
	if k := config.ReadConceptKey(cfg.RootPath, rel); k != "" {
		return k, nil
	}
	return "", fmt.Errorf("not a concept: %s", ref)
}
