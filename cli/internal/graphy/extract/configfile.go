package extract

import (
	"bytes"
	"fmt"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"okfw/cli/internal/graph"
	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func init() {
	register("yaml", func() (Extractor, error) { return &configExtractor{lang: "yaml"}, nil })
	register("json", func() (Extractor, error) { return &configExtractor{lang: "json"}, nil })
}

const (
	// maxConfigEntries caps entry nodes per file so lock files and other
	// machine-generated giants stay one file node instead of thousands.
	maxConfigEntries = 64
	// maxConfigRefs caps path-reference candidates per file.
	maxConfigRefs = 200
)

// configExtractor handles YAML and JSON in pure Go via yaml.v3 (JSON is a
// YAML subset, and the decoder reports line numbers for both). Config files
// aren't AST-shaped like code; what earns graph nodes is structure people
// navigate by — top-level keys (and their immediate children), Kubernetes
// manifests as whole resources — plus the highest-value edges configs carry:
// string values that reference repo files (CI scripts, Dockerfiles, includes).
type configExtractor struct {
	lang string
}

func (c *configExtractor) Language() string { return c.lang }
func (c *configExtractor) Close()           {}

func (c *configExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	fn := fileNode(c.lang, file)
	res.Nodes = append(res.Nodes, fn)

	// Parse failures (Helm templates, jinja-laced YAML) degrade to a bare
	// file node — never an error.
	docs := decodeDocs(src)

	entries := 0
	seenIDs := map[string]bool{}
	addNode := func(n model.Node, parentID string) string {
		if entries >= maxConfigEntries || seenIDs[n.ID] {
			return ""
		}
		seenIDs[n.ID] = true
		entries++
		res.Nodes = append(res.Nodes, n)
		res.Edges = append(res.Edges, containsEdge(parentID, n.ID, file.Path, n.Span.StartLine))
		return n.ID
	}

	// topAncestor maps a top-level yaml node (by pointer) to the entry node
	// refs beneath it are attributed to.
	type docEntry struct {
		value *yaml.Node
		id    string
	}
	var attributions []docEntry

	for i, doc := range docs {
		// Ansible playbooks are sequences of plays — the semantic layer
		// turns each play into a node and resolves its role references.
		if plays := ansiblePlays(doc); c.lang == "yaml" && len(plays) > 0 {
			for pi, p := range plays {
				label, qual := playLabel(p, pi)
				if i > 0 {
					qual = fmt.Sprintf("doc%d.%s", i, qual)
				}
				id := addNode(model.Node{
					ID:       model.NodeID(c.lang, model.KindConfig, file.Path, qual),
					Label:    label,
					Kind:     model.KindConfig,
					Language: c.lang,
					File:     file.Path,
					Span:     model.Span{StartLine: p.node.Line},
				}, fn.ID)
				if id == "" {
					continue
				}
				attributions = append(attributions, docEntry{value: p.node, id: id})
				ansibleRoleRefs(p.node, id, &res.Refs)
			}
			continue
		}
		// Task-list files (roles/*/tasks/*.yml) are sequences too: no entry
		// nodes, but include_role/import_role still resolve.
		if c.lang == "yaml" && doc.Kind == yaml.SequenceNode {
			ansibleRoleRefs(doc, fn.ID, &res.Refs)
			continue
		}
		if kind, name, ok := k8sManifest(doc); ok {
			label := kind
			if name != "" {
				label = kind + "/" + name
			}
			qual := label
			if i > 0 {
				qual = fmt.Sprintf("doc%d:%s", i, label)
			}
			id := addNode(model.Node{
				ID:       model.NodeID(c.lang, model.KindConfig, file.Path, qual),
				Label:    label,
				Kind:     model.KindConfig,
				Language: c.lang,
				File:     file.Path,
				Span:     model.Span{StartLine: doc.Line},
			}, fn.ID)
			if id != "" {
				attributions = append(attributions, docEntry{value: doc, id: id})
			}
			continue
		}
		if doc.Kind != yaml.MappingNode {
			continue
		}
		for k := 0; k+1 < len(doc.Content); k += 2 {
			keyNode, valNode := doc.Content[k], doc.Content[k+1]
			if keyNode.Kind != yaml.ScalarNode || keyNode.Value == "" {
				continue
			}
			qual := keyNode.Value
			if i > 0 {
				qual = fmt.Sprintf("doc%d.%s", i, keyNode.Value)
			}
			id := addNode(model.Node{
				ID:       model.NodeID(c.lang, model.KindConfig, file.Path, qual),
				Label:    keyNode.Value,
				Kind:     model.KindConfig,
				Language: c.lang,
				File:     file.Path,
				Span:     model.Span{StartLine: keyNode.Line},
			}, fn.ID)
			if id == "" {
				continue
			}
			attributions = append(attributions, docEntry{value: valNode, id: id})
			// One more level: children of a mapping value (compose services,
			// package.json scripts, ...).
			if valNode.Kind == yaml.MappingNode {
				for m := 0; m+1 < len(valNode.Content); m += 2 {
					childKey := valNode.Content[m]
					if childKey.Kind != yaml.ScalarNode || childKey.Value == "" {
						continue
					}
					addNode(model.Node{
						ID:       model.NodeID(c.lang, model.KindConfig, file.Path, qual+"."+childKey.Value),
						Label:    keyNode.Value + "." + childKey.Value,
						Kind:     model.KindConfig,
						Language: c.lang,
						File:     file.Path,
						Span:     model.Span{StartLine: childKey.Line},
					}, id)
				}
			}
		}
	}

	// Reference pass: every string scalar that could name a repo file becomes
	// a references ref, attributed to its top-level entry (or the file).
	// Resolution keeps only candidates that actually exist in the graph, so
	// the heuristic can stay loose.
	fromDir := ""
	if i := strings.LastIndex(file.Path, "/"); i >= 0 {
		fromDir = file.Path[:i]
	}
	seenRefs := map[string]bool{}
	visited := map[*yaml.Node]bool{}
	refs := 0
	collect := func(fromID string) func(*yaml.Node) {
		return func(n *yaml.Node) {
			if visited[n] || refs >= maxConfigRefs {
				return
			}
			visited[n] = true
			for _, cand := range pathCandidates(n.Value, fromDir) {
				key := fromID + "\x00" + cand
				if seenRefs[key] {
					continue
				}
				seenRefs[key] = true
				refs++
				res.Refs = append(res.Refs, model.Ref{
					FromID:   fromID,
					Name:     cand,
					Relation: model.RelReferences,
					Line:     n.Line,
				})
			}
		}
	}
	// Entry subtrees first (refs attributed to their entry), then anything
	// left in the documents (attributed to the file).
	for _, a := range attributions {
		walkScalars(a.value, collect(a.id))
	}
	for _, doc := range docs {
		walkScalars(doc, collect(fn.ID))
	}
	return res, nil
}

// decodeDocs parses a (possibly multi-document) YAML/JSON stream, returning
// the document content nodes decoded so far when parsing fails midway.
func decodeDocs(src []byte) []*yaml.Node {
	var docs []*yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(src))
	for {
		var doc yaml.Node
		if err := dec.Decode(&doc); err != nil {
			return docs
		}
		if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
			docs = append(docs, doc.Content[0])
		}
	}
}

// k8sManifest reports whether a document is a Kubernetes-style resource
// (apiVersion + kind), returning its kind and metadata.name.
func k8sManifest(doc *yaml.Node) (kind, name string, ok bool) {
	if doc.Kind != yaml.MappingNode {
		return "", "", false
	}
	apiVersion := mapScalar(doc, "apiVersion")
	kind = mapScalar(doc, "kind")
	if apiVersion == "" || kind == "" {
		return "", "", false
	}
	if meta := mapValue(doc, "metadata"); meta != nil {
		name = mapScalar(meta, "name")
	}
	return kind, name, true
}

func mapValue(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func mapScalar(mapping *yaml.Node, key string) string {
	if v := mapValue(mapping, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// walkScalars visits every scalar value node in the tree.
func walkScalars(n *yaml.Node, visit func(*yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.ScalarNode {
		visit(n)
		return
	}
	for _, c := range n.Content {
		walkScalars(c, visit)
	}
}

// pathCandidates interprets a scalar as a possible repo-file reference,
// returning its repo-root-relative and file-relative readings (deduplicated).
// Resolution validates existence, so false positives cost nothing but a
// dropped-ref stat.
func pathCandidates(value, fromDir string) []string {
	v := strings.TrimSpace(value)
	if v == "" || len(v) > 200 {
		return nil
	}
	if strings.ContainsAny(v, " \t\n") || strings.Contains(v, "://") {
		return nil
	}
	if strings.HasPrefix(v, "$") || strings.HasPrefix(v, "{{") || strings.HasPrefix(v, "/") {
		return nil // env/template expressions and absolute host paths
	}
	if !strings.ContainsAny(v, "/.") {
		return nil // bare words are never repo paths
	}
	var out []string
	add := func(p string) {
		if p == "" || p == "." || strings.HasPrefix(p, "../") {
			return
		}
		for _, seen := range out {
			if seen == p {
				return
			}
		}
		out = append(out, p)
	}
	// Repo-root-relative reading.
	add(strings.TrimPrefix(v, "./"))
	// File-relative reading (handles ./ and ../ against the config's dir).
	add(graph.NormalizeTarget(v, fromDir))
	return out
}
