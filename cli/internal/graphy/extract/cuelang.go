package extract

import (
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func init() {
	register("cue", func() (Extractor, error) { return &cueExtractor{}, nil })
}

// cueExtractor parses CUE schemas in pure Go — cuelang.org/go is already a
// dependency (okfw's concept/edge schema validation). Definitions (#Foo)
// become type nodes, plain top-level fields become config entries, imports
// become import refs, and identifier references between definitions
// (#Base embedded in #Derived) become references refs.
type cueExtractor struct{}

func (c *cueExtractor) Language() string { return "cue" }
func (c *cueExtractor) Close()           {}

func (c *cueExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	fn := fileNode("cue", file)
	res.Nodes = append(res.Nodes, fn)

	f, err := parser.ParseFile(file.Path, src, parser.ParseComments)
	if f == nil {
		// Unreadable source: degrade to the bare file node like the other
		// config-shaped extractors.
		_ = err
		return res, nil
	}

	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if path == "" {
			continue
		}
		res.Refs = append(res.Refs, model.Ref{
			FromID:   fn.ID,
			Name:     path,
			Relation: model.RelImports,
			Line:     spec.Pos().Line(),
		})
	}

	for _, decl := range f.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil || name == "" {
			continue
		}
		kind := model.KindConfig
		if strings.HasPrefix(name, "#") {
			kind = model.KindType // a CUE definition
		}
		n := model.Node{
			ID:        model.NodeID("cue", kind, file.Path, name),
			Label:     name,
			Kind:      kind,
			Language:  "cue",
			File:      file.Path,
			Span:      model.Span{StartLine: field.Pos().Line()},
			Signature: cueSignature(src, field),
			Doc:       cueDoc(field),
		}
		res.Nodes = append(res.Nodes, n)
		res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, n.Span.StartLine))

		// References to other definitions from this field's value.
		seen := map[string]bool{}
		ast.Walk(field.Value, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			ref := ident.Name
			if !strings.HasPrefix(ref, "#") || ref == name || seen[ref] {
				return true
			}
			seen[ref] = true
			res.Refs = append(res.Refs, model.Ref{
				FromID:   n.ID,
				Name:     ref,
				Relation: model.RelReferences,
				Line:     ident.Pos().Line(),
			})
			return true
		}, nil)
	}
	return res, nil
}

// cueSignature slices the field's first source line ("#Concept: {").
func cueSignature(src []byte, field *ast.Field) string {
	off := field.Pos().Offset()
	if off < 0 || off >= len(src) {
		return ""
	}
	line := string(src[off:])
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(line)
}

// cueDoc returns the first line of the field's doc comment.
func cueDoc(field *ast.Field) string {
	for _, group := range field.Comments() {
		if group == nil || len(group.List) == 0 {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(group.List[0].Text, "//"))
		if text != "" {
			return text
		}
	}
	return ""
}
