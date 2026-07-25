package extract

import (
	"fmt"
	"strings"

	ts "github.com/tree-sitter/go-tree-sitter"

	"okfw/cli/internal/graphy/model"
)

// tsBase wraps the C objects one tree-sitter extractor owns. Parsers and
// queries are not goroutine-safe; tsBase is confined to its Session.
type tsBase struct {
	lang    string
	parser  *ts.Parser
	query   *ts.Query
	capture map[uint32]string // capture index → capture name
}

func newTSBase(lang string, language *ts.Language, querySource string) (*tsBase, error) {
	query, qerr := ts.NewQuery(language, querySource)
	if qerr != nil {
		return nil, fmt.Errorf("%s query: %s", lang, qerr.Error())
	}
	parser := ts.NewParser()
	if err := parser.SetLanguage(language); err != nil {
		query.Close()
		parser.Close()
		return nil, fmt.Errorf("%s language: %w", lang, err)
	}
	names := map[uint32]string{}
	for i, name := range query.CaptureNames() {
		names[uint32(i)] = name
	}
	return &tsBase{lang: lang, parser: parser, query: query, capture: names}, nil
}

func (b *tsBase) Close() {
	if b.query != nil {
		b.query.Close()
	}
	if b.parser != nil {
		b.parser.Close()
	}
}

// match is one query match with captures grouped by capture name.
type match struct {
	pattern  uint
	captures map[string]ts.Node
}

// matches parses src and returns all query matches. The returned tree must
// stay alive while nodes are used; the caller closes it.
func (b *tsBase) matches(src []byte) (*ts.Tree, []match, error) {
	tree := b.parser.Parse(src, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("%s: parse returned no tree", b.lang)
	}
	cursor := ts.NewQueryCursor()
	defer cursor.Close()

	var out []match
	qm := cursor.Matches(b.query, tree.RootNode(), src)
	for m := qm.Next(); m != nil; m = qm.Next() {
		grouped := match{pattern: m.PatternIndex, captures: map[string]ts.Node{}}
		for _, c := range m.Captures {
			grouped.captures[b.capture[c.Index]] = c.Node
		}
		out = append(out, grouped)
	}
	return tree, out, nil
}

// spanOf converts a node's position to a 1-based model.Span.
func spanOf(n *ts.Node) model.Span {
	start, end := n.StartPosition(), n.EndPosition()
	return model.Span{
		StartLine: int(start.Row) + 1,
		StartCol:  int(start.Column) + 1,
		EndLine:   int(end.Row) + 1,
		EndCol:    int(end.Column) + 1,
	}
}

// firstDocLine returns the first line of a doc comment attached immediately
// above n (or its named ancestor), stripped of comment markers.
func firstDocLine(n *ts.Node, src []byte) string {
	prev := n.PrevNamedSibling()
	if prev == nil || prev.Kind() != "comment" {
		return ""
	}
	// The comment must end on the line directly above the declaration.
	if prev.EndPosition().Row+1 != n.StartPosition().Row {
		return ""
	}
	// Walk back to the first comment of a contiguous block.
	first := prev
	for {
		p := first.PrevNamedSibling()
		if p == nil || p.Kind() != "comment" || p.EndPosition().Row+1 != first.StartPosition().Row {
			break
		}
		first = p
	}
	line := first.Utf8Text(src)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimPrefix(line, "//")
	line = strings.TrimPrefix(line, "/*")
	return strings.TrimSpace(line)
}

// signatureOf returns the definition's header: its text up to the body field
// when present, else its first line.
func signatureOf(n *ts.Node, src []byte) string {
	text := n.Utf8Text(src)
	if body := n.ChildByFieldName("body"); body != nil {
		headLen := int(body.StartByte() - n.StartByte())
		if headLen > 0 && headLen <= len(text) {
			text = text[:headLen]
		}
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}

// enclosingDef walks up from n to the nearest ancestor whose kind is in
// defKinds, returning nil when none exists.
func enclosingDef(n *ts.Node, defKinds map[string]bool) *ts.Node {
	for p := n.Parent(); p != nil; p = p.Parent() {
		if defKinds[p.Kind()] {
			return p
		}
	}
	return nil
}
