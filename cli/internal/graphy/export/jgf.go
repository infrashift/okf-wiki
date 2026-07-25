package export

// JSON Graph Format output (https://jsongraphformat.info), v2.
//
// JGF keeps a small fixed vocabulary — nodes keyed by id, edges with
// source/target/relation — and pushes everything domain-specific into
// "metadata" objects, which is where graphy's kind, span, confidence and
// community land.

import (
	"encoding/json"
	"io"

	"okfw/cli/internal/graphy/model"
)

// JGFType is the graph "type" discriminator: JGF leaves the value to the
// producer, and a consumer holding several documents needs to know which ones
// carry graphy's metadata shape.
const JGFType = "graphy-code-graph"

// JGFSchemaVersion tracks model.SchemaVersion — the metadata bodies are the
// same vocabulary graph.json publishes, so they version together.
const JGFSchemaVersion = model.SchemaVersion

type jgfDocument struct {
	Graph jgfGraph `json:"graph"`
}

type jgfGraph struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Label    string             `json:"label"`
	Directed bool               `json:"directed"`
	Metadata jgfGraphMetadata   `json:"metadata"`
	Nodes    map[string]jgfNode `json:"nodes"`
	Edges    []jgfEdge          `json:"edges"`
}

type jgfGraphMetadata struct {
	SchemaVersion int            `json:"schema_version"`
	Generator     string         `json:"generator,omitempty"`
	Root          string         `json:"root"`
	Stats         model.Stats    `json:"stats"`
	Communities   []jgfCommunity `json:"communities,omitempty"`
}

// jgfCommunity restates the partition at the top of the document. Node
// metadata carries the denormalized community label already, but a consumer
// that wants to colour or group by subsystem should not have to reconstruct
// the list by scanning every node.
type jgfCommunity struct {
	ID    int    `json:"id"`
	Label string `json:"label,omitempty"`
	Size  int    `json:"size"`
}

type jgfNode struct {
	Label    string          `json:"label"`
	Metadata jgfNodeMetadata `json:"metadata"`
}

type jgfNodeMetadata struct {
	Kind           string      `json:"kind"`
	Language       string      `json:"language,omitempty"`
	SourceFile     string      `json:"source_file,omitempty"`
	SourceLocation *model.Span `json:"source_location,omitempty"`
	Signature      string      `json:"signature,omitempty"`
	Doc            string      `json:"doc,omitempty"`
	Body           string      `json:"body,omitempty"`
	Community      *int        `json:"community,omitempty"`
	CommunityLabel string      `json:"community_label,omitempty"`
	Degree         int         `json:"degree"`
	External       bool        `json:"external,omitempty"`
}

type jgfEdge struct {
	Source   string          `json:"source"`
	Target   string          `json:"target"`
	Relation string          `json:"relation"`
	Directed bool            `json:"directed"`
	Metadata jgfEdgeMetadata `json:"metadata"`
}

type jgfEdgeMetadata struct {
	Confidence string `json:"confidence"`
	SiteFile   string `json:"site_file,omitempty"`
	SiteLine   int    `json:"site_line,omitempty"`
}

// WriteJGF writes the snapshot as a JSON Graph Format v2 document.
//
// Like the GraphML writer it stamps no timestamp, so an unchanged graph
// re-exports byte for byte identically. HTML escaping is off: Go would
// otherwise turn every `<` in a signature into <, which is valid JSON but
// unreadable in a document full of generics and comparisons.
func WriteJGF(w io.Writer, g *Graph, generator string) error {
	doc := jgfDocument{Graph: jgfGraph{
		ID:       g.Root,
		Type:     JGFType,
		Label:    "graphy code graph of " + g.Root,
		Directed: true,
		Metadata: jgfGraphMetadata{
			SchemaVersion: JGFSchemaVersion,
			Generator:     generator,
			Root:          g.Root,
			Stats:         g.Stats(),
		},
		Nodes: make(map[string]jgfNode, len(g.Nodes)),
	}}
	for _, c := range g.Communities {
		doc.Graph.Metadata.Communities = append(doc.Graph.Metadata.Communities,
			jgfCommunity{ID: c.ID, Label: c.Label, Size: len(c.NodeIDs)})
	}

	for _, n := range g.Nodes {
		meta := jgfNodeMetadata{
			Kind:       string(n.Kind),
			Language:   n.Language,
			SourceFile: n.File,
			Signature:  n.Signature,
			Doc:        n.Doc,
			Body:       n.Body,
			Degree:     n.Degree,
			External:   n.External,
		}
		if n.Span != (model.Span{}) {
			span := n.Span
			meta.SourceLocation = &span
		}
		if id, label, ok := g.Community(n.ID); ok {
			community := id
			meta.Community = &community
			meta.CommunityLabel = label
		}
		doc.Graph.Nodes[n.ID] = jgfNode{Label: n.Label, Metadata: meta}
	}

	for _, e := range g.Edges {
		doc.Graph.Edges = append(doc.Graph.Edges, jgfEdge{
			Source:   e.Source,
			Target:   e.Target,
			Relation: string(e.Relation),
			Directed: true,
			Metadata: jgfEdgeMetadata{
				Confidence: string(e.Confidence),
				SiteFile:   e.File,
				SiteLine:   e.Line,
			},
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(doc)
}
