package extract

import (
	_ "embed"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

//go:embed queries/python.scm
var pythonQuery string

func init() {
	register("python", func() (Extractor, error) { return newPythonExtractor() })
}

var pyDefKinds = map[string]bool{
	"function_definition": true,
	"class_definition":    true,
}

type pythonExtractor struct {
	base *tsBase
}

func newPythonExtractor() (*pythonExtractor, error) {
	base, err := newTSBase("python", ts.NewLanguage(tspython.Language()), pythonQuery)
	if err != nil {
		return nil, err
	}
	return &pythonExtractor{base: base}, nil
}

func (p *pythonExtractor) Language() string { return "python" }
func (p *pythonExtractor) Close()           { p.base.Close() }

func (p *pythonExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	tree, matches, err := p.base.matches(src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	res := &Result{}
	fn := fileNode("python", file)
	res.Nodes = append(res.Nodes, fn)

	defByStart := map[uint]string{}
	for _, m := range matches {
		var def, name ts.Node
		var kind model.Kind
		switch {
		case has(m, "def.function"):
			def, name = m.captures["def.function"], m.captures["def.function.name"]
			kind = model.KindFunction
			if enclosingDef(&def, map[string]bool{"class_definition": true}) != nil {
				kind = model.KindMethod
			}
		case has(m, "def.class"):
			def, name = m.captures["def.class"], m.captures["def.class.name"]
			kind = model.KindClass
		default:
			continue
		}
		qual := pyQualName(&def, name.Utf8Text(src), src)
		n := model.Node{
			ID:        model.NodeID("python", kind, file.Path, qual),
			Label:     qual,
			Kind:      kind,
			Language:  "python",
			File:      file.Path,
			Span:      spanOf(&def),
			Signature: signatureOf(&def, src),
			Doc:       pyDocstring(&def, src),
		}
		defByStart[def.StartByte()] = n.ID
		res.Nodes = append(res.Nodes, n)
		res.Edges = append(res.Edges, containsEdge(fn.ID, n.ID, file.Path, n.Span.StartLine))
	}

	for _, m := range matches {
		var node ts.Node
		var rel model.Relation
		switch {
		case has(m, "ref.call"):
			node, rel = m.captures["ref.call"], model.RelCalls
		case has(m, "ref.import"):
			node, rel = m.captures["ref.import"], model.RelImports
		case has(m, "ref.inherit"):
			node, rel = m.captures["ref.inherit"], model.RelInherits
		default:
			continue
		}
		fromID := fn.ID
		if rel != model.RelImports {
			if def := enclosingDef(&node, pyDefKinds); def != nil {
				if id, ok := defByStart[def.StartByte()]; ok {
					fromID = id
				}
			}
		}
		res.Refs = append(res.Refs, model.Ref{
			FromID:   fromID,
			Name:     node.Utf8Text(src),
			Relation: rel,
			Line:     int(node.StartPosition().Row) + 1,
		})
	}
	return res, nil
}

// pyQualName prefixes a definition's name with its enclosing class/function
// chain: methods become "Class.method", nested defs "outer.inner".
func pyQualName(def *ts.Node, name string, src []byte) string {
	var chain []string
	for p := def.Parent(); p != nil; p = p.Parent() {
		if pyDefKinds[p.Kind()] {
			if n := p.ChildByFieldName("name"); n != nil {
				chain = append([]string{n.Utf8Text(src)}, chain...)
			}
		}
	}
	if len(chain) == 0 {
		return name
	}
	return strings.Join(chain, ".") + "." + name
}

// pyDocstring returns the first line of a def's docstring, when present.
func pyDocstring(def *ts.Node, src []byte) string {
	body := def.ChildByFieldName("body")
	if body == nil || body.NamedChildCount() == 0 {
		return ""
	}
	first := body.NamedChild(0)
	if first.Kind() != "expression_statement" || first.NamedChildCount() == 0 {
		return ""
	}
	str := first.NamedChild(0)
	if str.Kind() != "string" {
		return ""
	}
	text := str.Utf8Text(src)
	text = strings.Trim(text, `"'`)
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}
