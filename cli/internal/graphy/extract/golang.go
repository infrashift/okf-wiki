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
			n.Returns = goFirstResultType(&def, src)
			n.Locals = goLocalTypes(&def, src)
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
			n.Recv = goReceiverName(&def, src)
			n.Returns = goFirstResultType(&def, src)
			n.Locals = goLocalTypes(&def, src)
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
			switch kind {
			case model.KindInterface:
				n.Members = goInterfaceMethods(def.ChildByFieldName("type"), src)
			case model.KindStruct:
				n.Fields = goStructFields(def.ChildByFieldName("type"), src)
			}
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

// goInterfaceMethods lists an interface's declared method names, in source
// order. Embedded interfaces are skipped: their method sets are not visible
// here, and guessing at them would claim implementations that do not hold.
//
// The names are what makes implements inference possible at all — Go's
// interface satisfaction is structural, so there is no declaration anywhere
// naming the implementor, and the only evidence is the two method sets.
func goInterfaceMethods(iface *ts.Node, src []byte) []string {
	if iface == nil || iface.Kind() != "interface_type" {
		return nil
	}
	var out []string
	for i := uint(0); i < iface.NamedChildCount(); i++ {
		child := iface.NamedChild(i)
		if child.Kind() != "method_elem" {
			continue // embedded interface, type constraint, or comment
		}
		if name := child.ChildByFieldName("name"); name != nil {
			out = append(out, name.Utf8Text(src))
		}
	}
	return out
}

// goStructFields maps a struct's field names to the bare name of each field's
// type. Embedded fields are recorded under their own type name, which is what
// Go promotion makes them addressable as.
//
// The type is reduced to a bare name — pointers, slices, arrays, maps and
// generic arguments are peeled, and a package qualifier is dropped: resolution
// looks the result up in a symbol index keyed by declared name, and
// `*engine.Engine` and `Engine` name the same declaration there. A field whose
// type is not a repo declaration simply fails that lookup and costs nothing.
func goStructFields(st *ts.Node, src []byte) map[string]string {
	if st == nil || st.Kind() != "struct_type" {
		return nil
	}
	list := st.ChildByFieldName("body")
	if list == nil {
		if list = st.NamedChild(0); list == nil || list.Kind() != "field_declaration_list" {
			return nil
		}
	}
	out := map[string]string{}
	for i := uint(0); i < list.NamedChildCount(); i++ {
		fd := list.NamedChild(i)
		if fd.Kind() != "field_declaration" {
			continue // comment
		}
		t := fd.ChildByFieldName("type")
		if t == nil {
			continue
		}
		typ := goBareTypeName(t.Utf8Text(src))
		if typ == "" {
			continue
		}
		var named bool
		for j := uint(0); j < fd.NamedChildCount(); j++ {
			if c := fd.NamedChild(j); c.Kind() == "field_identifier" {
				out[c.Utf8Text(src)] = typ
				named = true
			}
		}
		// An embedded field has no field_identifier; it is reached by its own
		// type name.
		if !named {
			out[typ] = typ
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// goBareTypeName reduces a type expression to the bare declared name:
// "*engine.Engine" → "Engine", "[]*Policy" → "Policy", "map[string]*Rule" →
// "Rule", "Store[T]" → "Store". Returns "" for types with no single name
// (func literals, channels of composites, anonymous structs).
func goBareTypeName(text string) string {
	text = strings.TrimSpace(text)
	// Take the value type of a map; everything else peels from the left.
	if rest, ok := strings.CutPrefix(text, "map["); ok {
		if i := strings.IndexByte(rest, ']'); i >= 0 {
			text = strings.TrimSpace(rest[i+1:])
		}
	}
	text = strings.TrimLeft(text, "*[]&")
	if i := strings.IndexByte(text, '['); i >= 0 {
		text = text[:i] // generic arguments
	}
	if i := strings.LastIndexByte(text, '.'); i >= 0 {
		text = text[i+1:] // package qualifier
	}
	text = strings.TrimSpace(text)
	for _, r := range text {
		if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return ""
		}
	}
	return text
}

// callPrefix marks a local whose type is "whatever F returns". Resolution
// substitutes the callee's Returns once every node is known, because the
// constructor usually lives in another file and extraction is per-file.
const callPrefix = "()"

// goFirstResultType returns the bare name of a function's first result type.
// First rather than all, because the Go constructor idiom is `(*T, error)` and
// the value that gets a method called on it is always the first one.
func goFirstResultType(def *ts.Node, src []byte) string {
	res := def.ChildByFieldName("result")
	if res == nil {
		return ""
	}
	// A single unnamed result is the type node itself; several are wrapped in
	// a parameter_list.
	if res.Kind() != "parameter_list" {
		return goBareTypeName(res.Utf8Text(src))
	}
	for i := uint(0); i < res.NamedChildCount(); i++ {
		p := res.NamedChild(i)
		if p.Kind() != "parameter_declaration" {
			continue
		}
		if t := p.ChildByFieldName("type"); t != nil {
			return goBareTypeName(t.Utf8Text(src))
		}
	}
	return ""
}

// goLocalTypes lists a function's variable declarations with the line range
// over which each one is the binding for its name, covering the forms that
// carry a type without inference:
//
//	func f(e *engine.Engine)     parameter        → Engine
//	var e *engine.Engine         typed var        → Engine
//	e := &Engine{} / Engine{}    composite literal→ Engine
//	e := NewEngine()             constructor call → "()NewEngine", resolved later
//
// Ranges rather than one map entry per name, because Go lets an inner scope
// rebind a name to a different type. Flattening that to "last declaration
// wins" mistypes every call in the *inner* scope whenever an outer
// redeclaration follows it in source order — and a mistyped receiver produces
// a confident wrong edge, which is worse than the drop it replaced.
func goLocalTypes(def *ts.Node, src []byte) []model.Local {
	var out []model.Local
	fnEnd := int(def.EndPosition().Row) + 1
	if params := def.ChildByFieldName("parameters"); params != nil {
		for i := uint(0); i < params.NamedChildCount(); i++ {
			p := params.NamedChild(i)
			if p.Kind() != "parameter_declaration" {
				continue
			}
			t := p.ChildByFieldName("type")
			if t == nil {
				continue
			}
			typ := goBareTypeName(t.Utf8Text(src))
			if typ == "" {
				continue
			}
			for j := uint(0); j < p.NamedChildCount(); j++ {
				if c := p.NamedChild(j); c.Kind() == "identifier" {
					// A parameter binds across the whole body.
					out = append(out, model.Local{
						Name:  c.Utf8Text(src),
						Type:  typ,
						Start: int(def.StartPosition().Row) + 1,
						End:   fnEnd,
					})
				}
			}
		}
	}
	if body := def.ChildByFieldName("body"); body != nil {
		goCollectLocals(body, src, fnEnd, &out)
	}
	return out
}

// goScopeKinds are the constructs that open a new binding scope. An `if` or
// `for` initializer binds beyond the braces it precedes, so the statement — not
// its block — is the scope.
var goScopeKinds = map[string]bool{
	"block":                       true,
	"if_statement":                true,
	"for_statement":               true,
	"expression_switch_statement": true,
	"type_switch_statement":       true,
	"select_statement":            true,
	"func_literal":                true,
}

func goCollectLocals(n *ts.Node, src []byte, scopeEnd int, out *[]model.Local) {
	switch n.Kind() {
	case "var_spec":
		if t := n.ChildByFieldName("type"); t != nil {
			if typ := goBareTypeName(t.Utf8Text(src)); typ != "" {
				for i := uint(0); i < n.NamedChildCount(); i++ {
					if c := n.NamedChild(i); c.Kind() == "identifier" {
						*out = append(*out, model.Local{
							Name: c.Utf8Text(src), Type: typ,
							Start: int(c.StartPosition().Row) + 1, End: scopeEnd,
						})
					}
				}
			}
		}
	case "short_var_declaration":
		left, right := n.ChildByFieldName("left"), n.ChildByFieldName("right")
		if left != nil && right != nil {
			for i := uint(0); i < left.NamedChildCount(); i++ {
				name := left.NamedChild(i)
				if name.Kind() != "identifier" {
					continue
				}
				// `v, err := f()` is one call feeding several names: only the
				// first takes the call's type. Otherwise pair positionally.
				var rhs *ts.Node
				switch {
				case right.NamedChildCount() == left.NamedChildCount():
					rhs = right.NamedChild(i)
				case right.NamedChildCount() == 1 && i == 0:
					rhs = right.NamedChild(0)
				}
				if rhs == nil {
					continue
				}
				if typ := goExprType(rhs, src); typ != "" {
					*out = append(*out, model.Local{
						Name: name.Utf8Text(src), Type: typ,
						Start: int(name.StartPosition().Row) + 1, End: scopeEnd,
					})
				}
			}
		}
	}
	for i := uint(0); i < n.NamedChildCount(); i++ {
		child := n.NamedChild(i)
		end := scopeEnd
		if goScopeKinds[child.Kind()] {
			end = int(child.EndPosition().Row) + 1
		}
		goCollectLocals(child, src, end, out)
	}
}

// goExprType names the type an expression evaluates to, for the two shapes that
// say so without a type checker: a composite literal, and a call whose callee
// is named (deferred as "()F").
func goExprType(n *ts.Node, src []byte) string {
	switch n.Kind() {
	case "composite_literal":
		if t := n.ChildByFieldName("type"); t != nil {
			return goBareTypeName(t.Utf8Text(src))
		}
	case "unary_expression", "parenthesized_expression":
		if op := n.ChildByFieldName("operand"); op != nil {
			return goExprType(op, src)
		}
		if n.NamedChildCount() > 0 {
			return goExprType(n.NamedChild(0), src)
		}
	case "call_expression":
		fn := n.ChildByFieldName("function")
		if fn == nil {
			return ""
		}
		switch fn.Kind() {
		case "identifier":
			return callPrefix + fn.Utf8Text(src)
		case "selector_expression":
			// `engine.NewEngine(...)`: the symbol index is keyed by declared
			// name, so the package qualifier is dropped here exactly as it is
			// for a field's type.
			if f := fn.ChildByFieldName("field"); f != nil {
				return callPrefix + f.Utf8Text(src)
			}
		}
	}
	return ""
}

// goReceiverName extracts a method's receiver variable name: "s" in
// "func (s *Server) Handle()". Empty for an unnamed receiver.
func goReceiverName(method *ts.Node, src []byte) string {
	recv := method.ChildByFieldName("receiver")
	if recv == nil || recv.NamedChildCount() == 0 {
		return ""
	}
	decl := recv.NamedChild(0)
	if name := decl.ChildByFieldName("name"); name != nil {
		return name.Utf8Text(src)
	}
	return ""
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
