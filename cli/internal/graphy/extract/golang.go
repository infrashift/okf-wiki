package extract

import (
	_ "embed"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

//go:embed queries/go.scm
var goQuery string

func init() {
	register("go", func() (Extractor, error) { return newGoExtractor() })
}

var goDefKinds = map[string]bool{
	"function_declaration": true,
	"method_declaration":   true,
}

type goExtractor struct {
	base *tsBase
}

func newGoExtractor() (*goExtractor, error) {
	base, err := newTSBase("go", ts.NewLanguage(tsgo.Language()), goQuery)
	if err != nil {
		return nil, err
	}
	return &goExtractor{base: base}, nil
}

func (g *goExtractor) Language() string { return "go" }
func (g *goExtractor) Close()           { g.base.Close() }

func (g *goExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	tree, matches, err := g.base.matches(src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	res := &Result{}
	fn := fileNode("go", file)
	res.Nodes = append(res.Nodes, fn)

	// Pass 1: definitions. defByStart maps a definition node's start byte to
	// its model id so references can be attributed to their enclosing def.
	defByStart := map[uint]string{}
	for _, m := range matches {
		switch {
		case has(m, "def.function"):
			def, name := m.captures["def.function"], m.captures["def.function.name"]
			qual := name.Utf8Text(src)
			n := g.defNode(model.KindFunction, qual, qual, &def, src, file)
			defByStart[def.StartByte()] = n.ID
			res.Nodes = append(res.Nodes, n)
			res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, n.Span.StartLine))

		case has(m, "def.method"):
			def, name := m.captures["def.method"], m.captures["def.method.name"]
			recv := goReceiverType(&def, src)
			qual := name.Utf8Text(src)
			if recv != "" {
				qual = recv + "." + qual
			}
			n := g.defNode(model.KindMethod, qual, qual, &def, src, file)
			defByStart[def.StartByte()] = n.ID
			res.Nodes = append(res.Nodes, n)
			res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, n.Span.StartLine))
			// A method structurally belongs to its receiver type. RelDefines
			// refs are reversed at resolution time: the resolved type becomes
			// the edge source (Type -defines-> method).
			if recv != "" {
				res.Refs = append(res.Refs, model.Ref{
					FromID:   n.ID,
					Name:     recv,
					Relation: model.RelDefines,
					Line:     n.Span.StartLine,
				})
			}

		case has(m, "def.type"):
			def, name := m.captures["def.type"], m.captures["def.type.name"]
			kind := model.KindType
			if t := def.ChildByFieldName("type"); t != nil {
				switch t.Kind() {
				case "struct_type":
					kind = model.KindStruct
				case "interface_type":
					kind = model.KindInterface
				}
			}
			qual := name.Utf8Text(src)
			n := g.defNode(kind, qual, qual, &def, src, file)
			defByStart[def.StartByte()] = n.ID
			res.Nodes = append(res.Nodes, n)
			res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, n.Span.StartLine))
		}
	}

	// Pass 2: references, attributed to their enclosing definition.
	for _, m := range matches {
		switch {
		case has(m, "ref.call"):
			call := m.captures["ref.call"]
			name := call.Utf8Text(src)
			fromID := fn.ID
			if def := enclosingDef(&call, goDefKinds); def != nil {
				if id, ok := defByStart[def.StartByte()]; ok {
					fromID = id
				}
			}
			res.Refs = append(res.Refs, model.Ref{
				FromID:   fromID,
				Name:     name,
				Relation: model.RelCalls,
				Line:     int(call.StartPosition().Row) + 1,
			})

		case has(m, "ref.import"):
			imp := m.captures["ref.import"]
			path := strings.Trim(imp.Utf8Text(src), `"`)
			res.Refs = append(res.Refs, model.Ref{
				FromID:   fn.ID,
				Name:     path,
				Relation: model.RelImports,
				Line:     int(imp.StartPosition().Row) + 1,
			})
		}
	}
	return res, nil
}

func (g *goExtractor) defNode(kind model.Kind, label, qual string, def *ts.Node, src []byte, file detect.File) model.Node {
	// Doc comments for type_specs hang off the enclosing type_declaration.
	docAnchor := def
	if def.Kind() == "type_spec" {
		if p := def.Parent(); p != nil && p.Kind() == "type_declaration" {
			docAnchor = p
		}
	}
	return model.Node{
		ID:        model.NodeID("go", kind, file.Path, qual),
		Label:     label,
		Kind:      kind,
		Language:  "go",
		File:      file.Path,
		Span:      spanOf(def),
		Signature: signatureOf(def, src),
		Doc:       firstDocLine(docAnchor, src),
	}
}

// goReceiverType extracts a method's receiver type name, stripping pointers
// and generic type parameters: "func (s *Store[T]) X()" → "Store".
func goReceiverType(method *ts.Node, src []byte) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil || recv.NamedChildCount() == 0 {
		return ""
	}
	decl := recv.NamedChild(0)
	t := decl.ChildByFieldName("type")
	if t == nil {
		return ""
	}
	text := t.Utf8Text(src)
	text = strings.TrimPrefix(text, "*")
	if i := strings.IndexByte(text, '['); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

func has(m match, name string) bool {
	_, ok := m.captures[name]
	return ok
}
