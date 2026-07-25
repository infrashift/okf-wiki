// Package model defines graphy's graph vocabulary: nodes, typed edges with
// confidence, unresolved references, and the portable graph.json encoding.
//
// Node IDs are deterministic strings ("<lang>:<kind>:<relfile>#<qualified-name>")
// so repeated builds and re-exports are stable without an id-mapping table.
package model

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// Kind classifies what a node represents.
type Kind string

const (
	KindFunction  Kind = "function"
	KindMethod    Kind = "method"
	KindClass     Kind = "class"
	KindStruct    Kind = "struct"
	KindInterface Kind = "interface"
	KindType      Kind = "type"
	KindModule    Kind = "module"
	KindFile      Kind = "file"
	KindDoc       Kind = "doc"
	KindHeading   Kind = "heading"
	KindVariable  Kind = "variable"
	KindConfig    Kind = "config" // a config-file entry (yaml/json key, manifest)
	KindExternal  Kind = "external"
)

// Relation is the typed relationship an edge carries.
type Relation string

const (
	RelCalls      Relation = "calls"
	RelImports    Relation = "imports"
	RelInherits   Relation = "inherits"
	RelImplements Relation = "implements"
	RelReferences Relation = "references"
	RelContains   Relation = "contains"
	RelDefines    Relation = "defines"
)

// Confidence tags how an edge was established.
type Confidence string

const (
	// ConfExtracted marks a relationship stated explicitly in source and
	// resolved unambiguously (same file/scope).
	ConfExtracted Confidence = "EXTRACTED"
	// ConfInferred marks a relationship resolved via a unique global symbol
	// match or a synthesized external target.
	ConfInferred Confidence = "INFERRED"
	// ConfAmbiguous marks a relationship with multiple plausible targets.
	ConfAmbiguous Confidence = "AMBIGUOUS"
)

// Span is a source location range (1-based lines).
type Span struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	EndCol    int `json:"end_col,omitempty"`
}

// Node is a code or documentation entity.
type Node struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Kind      Kind   `json:"kind"`
	Language  string `json:"language,omitempty"`
	File      string `json:"source_file,omitempty"` // repo-relative, slash-separated
	Span      Span   `json:"source_location"`
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"` // first doc-comment line
	Community int    `json:"community"`
	Degree    int    `json:"degree"`
	External  bool   `json:"external,omitempty"` // unresolved import target
}

// Edge is a typed, confidence-tagged relationship between two nodes.
type Edge struct {
	Source     string     `json:"source"`
	Target     string     `json:"target"`
	Relation   Relation   `json:"relation"`
	Confidence Confidence `json:"confidence"`
	File       string     `json:"site_file,omitempty"` // where the reference occurs
	Line       int        `json:"site_line,omitempty"`
}

// Ref is an unresolved cross-file reference produced by extraction and
// resolved into edges by the build's second pass.
type Ref struct {
	FromID   string   `json:"from"`
	Name     string   `json:"name"` // possibly qualified: "pkg.Func", "Base"
	Relation Relation `json:"relation"`
	Line     int      `json:"line,omitempty"`
}

// NodeID builds the deterministic node id for an entity.
func NodeID(lang string, kind Kind, relFile, qualName string) string {
	return fmt.Sprintf("%s:%s:%s#%s", lang, kind, relFile, qualName)
}

// Stats summarizes a graph for reports and graph.json.
type Stats struct {
	Nodes       int `json:"nodes"`
	Edges       int `json:"edges"`
	Communities int `json:"communities"`
	Files       int `json:"files"`
}

// GraphJSON is the portable on-disk graph representation.
type GraphJSON struct {
	SchemaVersion int    `json:"schema_version"`
	GeneratedAt   string `json:"generated_at"`
	Root          string `json:"root"`
	Stats         Stats  `json:"stats"`
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

// SchemaVersion is the current graph.json schema version.
const SchemaVersion = 1

// EncodeGraphJSON writes a deterministic (sorted) graph.json document.
func EncodeGraphJSON(w io.Writer, root string, nodes []Node, edges []Edge, communities int, files int) error {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Relation != b.Relation {
			return a.Relation < b.Relation
		}
		return a.Target < b.Target
	})
	doc := GraphJSON{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Root:          root,
		Stats: Stats{
			Nodes:       len(nodes),
			Edges:       len(edges),
			Communities: communities,
			Files:       files,
		},
		Nodes: nodes,
		Edges: edges,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
