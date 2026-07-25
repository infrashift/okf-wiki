// Package extract turns source files into graph fragments: definition nodes,
// fully-resolved intra-file edges, and unresolved cross-file references.
//
// Code languages use deterministic tree-sitter AST extraction (no LLM
// involvement); markdown is handled in pure Go. Each extractor guarantees its
// Result's first node is the file-level node, which cross-file resolution
// uses as the target for path-shaped references.
package extract

import (
	"fmt"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

// Result is one file's extraction output.
type Result struct {
	Nodes []model.Node // Nodes[0] is always the file node
	Edges []model.Edge // intra-file, fully resolved
	Refs  []model.Ref  // unresolved cross-file references
}

// Extractor extracts one language.
type Extractor interface {
	Language() string
	Extract(file detect.File, src []byte) (*Result, error)
	// Close releases any C resources (tree-sitter parsers/queries).
	Close()
}

// Session owns one extractor per language. Extractors hold tree-sitter
// parsers, which are not goroutine-safe: use one Session per worker and
// Close it when done.
type Session struct {
	extractors map[string]Extractor
	opts       SessionOptions
}

// SessionOptions configures extraction passes that are not language-specific.
type SessionOptions struct {
	Rationale RationaleOptions
}

// NewSession builds extractors for the requested languages (all supported
// languages when langs is empty), with rationale extraction enabled.
func NewSession(langs []string) (*Session, error) {
	return NewSessionWith(langs, SessionOptions{Rationale: DefaultRationaleOptions()})
}

// NewSessionWith is NewSession with explicit non-language options.
func NewSessionWith(langs []string, opts SessionOptions) (*Session, error) {
	want := map[string]bool{}
	for _, l := range langs {
		want[l] = true
	}
	s := &Session{extractors: map[string]Extractor{}, opts: opts}
	for lang, factory := range factories {
		if len(want) > 0 && !want[lang] {
			continue
		}
		ex, err := factory()
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("initializing %s extractor: %w", lang, err)
		}
		s.extractors[lang] = ex
	}
	return s, nil
}

// Supported reports whether the session has an extractor for the language.
func (s *Session) Supported(lang string) bool {
	_, ok := s.extractors[lang]
	return ok
}

// Extract runs the file's language extractor, then the language-agnostic
// rationale pass.
//
// Rationale lives here rather than inside each Extractor for three reasons:
// it covers all languages without touching eleven files, it needs one toggle
// rather than eleven, and the ordering constraint is satisfied for free —
// the Result already holds the file's definition nodes and their spans, which
// is exactly what attaching a comment to its owner requires.
func (s *Session) Extract(file detect.File, src []byte) (*Result, error) {
	ex, ok := s.extractors[file.Language]
	if !ok {
		return nil, fmt.Errorf("no extractor for language %q", file.Language)
	}
	res, err := ex.Extract(file, src)
	if err != nil {
		return nil, err
	}
	if s.opts.Rationale.Enabled {
		attachRationale(file, src, res, s.opts.Rationale)
	}
	return res, nil
}

// Close releases all extractor resources.
func (s *Session) Close() {
	for _, ex := range s.extractors {
		ex.Close()
	}
	s.extractors = nil
}

// factories registers the available extractor constructors by language name.
var factories = map[string]func() (Extractor, error){}

func register(lang string, factory func() (Extractor, error)) {
	factories[lang] = factory
}

// Languages returns the registered language names.
func Languages() []string {
	out := make([]string, 0, len(factories))
	for lang := range factories {
		out = append(out, lang)
	}
	return out
}

// fileNode builds the standard file-level node every extractor emits first.
func fileNode(lang string, file detect.File) model.Node {
	return model.Node{
		ID:       model.NodeID(lang, model.KindFile, file.Path, ""),
		Label:    file.Path,
		Kind:     model.KindFile,
		Language: lang,
		File:     file.Path,
		Span:     model.Span{StartLine: 1},
	}
}

// containsEdge links a file node to a definition it contains.
func containsEdge(fileID, defID, relPath string, line int) model.Edge {
	return model.Edge{
		Source:     fileID,
		Target:     defID,
		Relation:   model.RelContains,
		Confidence: model.ConfExtracted,
		File:       relPath,
		Line:       line,
	}
}
