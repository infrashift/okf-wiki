package extract

import (
	_ "embed"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tstsx "github.com/tree-sitter/tree-sitter-typescript/bindings/go"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

//go:embed queries/javascript.scm
var javascriptQuery string

//go:embed queries/typescript.scm
var typescriptQuery string

func init() {
	register("javascript", func() (Extractor, error) {
		return newScriptExtractor("javascript", ts.NewLanguage(tsjs.Language()), javascriptQuery, nil)
	})
	register("typescript", func() (Extractor, error) {
		// One extractor serves .ts via the typescript grammar and .tsx via
		// the tsx grammar; both compile the same query.
		tsx, err := newTSBase("typescript", ts.NewLanguage(tstsx.LanguageTSX()), typescriptQuery)
		if err != nil {
			return nil, err
		}
		return newScriptExtractor("typescript", ts.NewLanguage(tstsx.LanguageTypescript()), typescriptQuery, tsx)
	})
}

var jsDefKinds = map[string]bool{
	"function_declaration":           true,
	"generator_function_declaration": true,
	"method_definition":              true,
	"variable_declarator":            true,
	"class_declaration":              true,
}

// scriptExtractor covers JavaScript and TypeScript (plus TSX via a second
// grammar) — the ECMAScript family shares its extraction logic.
type scriptExtractor struct {
	lang string
	base *tsBase
	tsx  *tsBase // typescript only: grammar for .tsx files
}

func newScriptExtractor(lang string, language *ts.Language, query string, tsx *tsBase) (*scriptExtractor, error) {
	base, err := newTSBase(lang, language, query)
	if err != nil {
		if tsx != nil {
			tsx.Close()
		}
		return nil, err
	}
	return &scriptExtractor{lang: lang, base: base, tsx: tsx}, nil
}

func (s *scriptExtractor) Language() string { return s.lang }

func (s *scriptExtractor) Close() {
	s.base.Close()
	if s.tsx != nil {
		s.tsx.Close()
	}
}

func (s *scriptExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	base := s.base
	if s.tsx != nil && strings.HasSuffix(strings.ToLower(file.Path), ".tsx") {
		base = s.tsx
	}
	tree, matches, err := base.matches(src)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	res := &Result{}
	fn := fileNode(s.lang, file)
	res.Nodes = append(res.Nodes, fn)

	defByStart := map[uint]string{}
	for _, m := range matches {
		var def, name ts.Node
		var kind model.Kind
		switch {
		case has(m, "def.function"):
			def, name, kind = m.captures["def.function"], m.captures["def.function.name"], model.KindFunction
		case has(m, "def.method"):
			def, name, kind = m.captures["def.method"], m.captures["def.method.name"], model.KindMethod
		case has(m, "def.class"):
			def, name, kind = m.captures["def.class"], m.captures["def.class.name"], model.KindClass
		case has(m, "def.interface"):
			def, name, kind = m.captures["def.interface"], m.captures["def.interface.name"], model.KindInterface
		case has(m, "def.type"):
			def, name, kind = m.captures["def.type"], m.captures["def.type.name"], model.KindType
		default:
			continue
		}
		qual := name.Utf8Text(src)
		if kind == model.KindMethod {
			if cls := jsEnclosingClassName(&def, src); cls != "" {
				qual = cls + "." + qual
			}
		}
		n := model.Node{
			ID:        model.NodeID(s.lang, kind, file.Path, qual),
			Label:     qual,
			Kind:      kind,
			Language:  s.lang,
			File:      file.Path,
			Span:      spanOf(&def),
			Signature: signatureOf(&def, src),
			Doc:       firstDocLine(jsStmtAnchor(&def), src),
		}
		// Anonymous defs (e.g. computed method names) are skipped.
		if strings.TrimSpace(name.Utf8Text(src)) == "" {
			continue
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
		case has(m, "ref.implement"):
			node, rel = m.captures["ref.implement"], model.RelImplements
		default:
			continue
		}
		name := node.Utf8Text(src)
		if rel == model.RelImports {
			name = strings.Trim(name, "`'\"")
		}
		fromID := fn.ID
		if rel != model.RelImports {
			if def := enclosingDef(&node, jsDefKinds); def != nil {
				if id, ok := defByStart[def.StartByte()]; ok {
					fromID = id
				}
			}
		}
		res.Refs = append(res.Refs, model.Ref{
			FromID:   fromID,
			Name:     name,
			Relation: rel,
			Line:     int(node.StartPosition().Row) + 1,
		})
	}
	return res, nil
}

// jsEnclosingClassName returns the name of the class a method belongs to.
func jsEnclosingClassName(def *ts.Node, src []byte) string {
	for p := def.Parent(); p != nil; p = p.Parent() {
		if p.Kind() == "class_declaration" || p.Kind() == "class" {
			if n := p.ChildByFieldName("name"); n != nil {
				return n.Utf8Text(src)
			}
			return ""
		}
	}
	return ""
}

// jsStmtAnchor climbs from a definition to the statement doc comments attach
// to: export statements and variable declarations wrap the definition node.
func jsStmtAnchor(def *ts.Node) *ts.Node {
	anchor := def
	for p := anchor.Parent(); p != nil; p = p.Parent() {
		switch p.Kind() {
		case "export_statement", "lexical_declaration", "variable_declaration":
			anchor = p
		default:
			return anchor
		}
	}
	return anchor
}
