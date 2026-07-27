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
	"slices"
	"sort"
	"strconv"
	"strings"
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
	KindConfig    Kind = "config"   // a config-file entry (yaml/json key, manifest)
	KindResource  Kind = "resource" // an infrastructure resource (terraform resource/data)
	KindExternal  Kind = "external"
	// KindRationale is recorded intent: a marked source comment (WHY:,
	// DECISION:, TODO: …) or an architecture decision record. It is the only
	// kind whose content is prose rather than code.
	KindRationale Kind = "rationale"
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
	// RelExplains points from a rationale to the entity it justifies. The
	// rationale is the source because Ref.FromID is the only end an unresolved
	// reference can pin, and ADR→code links must resolve through refs; keeping
	// one direction for the whole kind means an entity's reasoning always
	// shows up under its incoming edges.
	RelExplains Relation = "explains"
	// RelTests points from a test to the entity it exercises. It is separate
	// from RelCalls because a test calling a function and a caller calling it
	// mean different things: one is a usage, the other is a specification, and
	// "what tests this?" is a question an agent asks constantly while "which of
	// my callers happens to be a test?" is not.
	RelTests Relation = "tests"
	// RelDependsOn points from a repository to a declared third-party
	// dependency, read from a package manifest rather than from source.
	RelDependsOn Relation = "depends_on"
)

// allRelations and allKinds back the validators below. A query that names a
// relation or kind gets an error rather than a silently empty filter, which
// would widen the query to everything and read as "no results".
var allRelations = []Relation{
	RelCalls, RelImports, RelInherits, RelImplements,
	RelReferences, RelContains, RelDefines, RelExplains, RelTests, RelDependsOn,
}

var allKinds = []Kind{
	KindFunction, KindMethod, KindClass, KindStruct, KindInterface, KindType,
	KindModule, KindFile, KindDoc, KindHeading, KindVariable, KindConfig,
	KindResource, KindExternal, KindRationale,
}

// ValidRelation reports whether r is part of the vocabulary.
func ValidRelation(r Relation) bool { return slices.Contains(allRelations, r) }

// ValidKind reports whether k is part of the vocabulary.
func ValidKind(k Kind) bool { return slices.Contains(allKinds, k) }

// RelationNames lists the relation vocabulary, for help text and errors.
func RelationNames() []string {
	out := make([]string, len(allRelations))
	for i, r := range allRelations {
		out[i] = string(r)
	}
	sort.Strings(out)
	return out
}

// KindNames lists the kind vocabulary, for help text and errors.
func KindNames() []string {
	out := make([]string, len(allKinds))
	for i, k := range allKinds {
		out[i] = string(k)
	}
	sort.Strings(out)
	return out
}

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
	// Body is free-form multi-line prose for nodes whose content IS text
	// (rationale, ADRs). Empty for code entities, whose text is Signature+Doc.
	// Doc stays single-line for everyone: it feeds concept descriptions,
	// terminal cards and search snippets, all of which want one line.
	Body      string `json:"body,omitempty"`
	Community int    `json:"community"`
	Degree    int    `json:"degree"`
	External  bool   `json:"external,omitempty"` // unresolved import target
	// Test marks an entity declared in a test file. This is worth recording
	// even where no `tests` edge could be derived: test files are a large
	// fraction of a repo, they inflate Degree (which --min-degree reads as
	// significance) and they pull the community partition toward mirroring the
	// test suite rather than the system under test.
	Test bool `json:"is_test,omitempty"`
	// Members lists an interface's method names, in declaration order. Only
	// interface nodes populate it; it exists so implementation matching can
	// compare method sets without re-parsing.
	Members []string `json:"members,omitempty"`
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

// rationaleSep separates the parts of a rationale's qualified name. It is '@'
// rather than '#' because '#' already separates file from qualname in a node
// id, and the export's id parser splits on the LAST '#'.
const rationaleSep = "@"

// RationaleQual builds a rationale node's qualified name from the qualified
// name of the entity it explains, the marker word, and its ordinal among the
// rationales sharing that owner and marker ("" owner = file-level).
//
// Deliberately free of line numbers and of the rationale's own text: comments
// get reworded and code moves down the file constantly, and either would
// otherwise destroy the identity — and with it the exported concept's id and
// creation date. Keying on the owner instead means a rationale survives edits
// to itself and follows its owner across renames and file moves.
func RationaleQual(ownerQual, marker string, ord int) string {
	return fmt.Sprintf("%s%s%s%s%d", ownerQual, rationaleSep, marker, rationaleSep, ord)
}

// ParseRationaleQual splits a qualified name built by RationaleQual.
func ParseRationaleQual(qual string) (owner, marker string, ord int, ok bool) {
	last := strings.LastIndex(qual, rationaleSep)
	if last < 0 {
		return "", "", 0, false
	}
	ord, err := strconv.Atoi(qual[last+len(rationaleSep):])
	if err != nil {
		return "", "", 0, false
	}
	head := qual[:last]
	prev := strings.LastIndex(head, rationaleSep)
	if prev < 0 {
		return "", "", 0, false
	}
	marker = head[prev+len(rationaleSep):]
	if marker == "" {
		return "", "", 0, false
	}
	return head[:prev], marker, ord, true
}

// nonIndexableKinds are the kinds excluded from the definition-symbol index:
// they are scaffolding or prose, never the target of a code reference.
var nonIndexableKinds = map[Kind]bool{
	KindFile:     true,
	KindDoc:      true,
	KindHeading:  true,
	KindConfig:   true,
	KindExternal: true,
	// Rationale labels are prose, never a call target. Indexing them would let
	// a code reference resolve onto a comment.
	KindRationale: true,
}

// Indexable reports whether a node belongs in the definition-symbol index.
// Both the store's index writer and the build's in-memory resolution index
// gate on this, so the two cannot drift apart.
func Indexable(n Node) bool {
	return !n.External && !nonIndexableKinds[n.Kind]
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

// SchemaVersion is the current graph.json schema version. Bumped to 2 when
// the rationale kind, the explains relation and Node.Body were added, and to 3
// for the tests and depends_on relations plus Node.Test and Node.Members — the
// document is advertised as portable, so new vocabulary is a contract change.
const SchemaVersion = 3

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
