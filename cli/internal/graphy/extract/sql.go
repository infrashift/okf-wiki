package extract

import (
	"regexp"
	"sort"
	"strings"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func init() {
	register("sql", func() (Extractor, error) { return &sqlExtractor{}, nil })
}

// sqlExtractor is a dependency-free DDL scanner. Full SQL grammars are
// dialect-specific and heavy; schema files are navigated by their CREATE
// statements, which a comment-stripped regex pass captures reliably across
// dialects. Tables and views become nodes; foreign keys (REFERENCES) and view
// sources (FROM/JOIN) become refs attributed to their enclosing statement,
// resolved through the symbol index like any other language.
type sqlExtractor struct{}

func (s *sqlExtractor) Language() string { return "sql" }
func (s *sqlExtractor) Close()           {}

var (
	sqlTableRe = regexp.MustCompile(`(?i)\bCREATE\s+(?:TEMP\s+|TEMPORARY\s+|UNLOGGED\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w".]+)`)
	sqlViewRe  = regexp.MustCompile(`(?i)\bCREATE\s+(?:OR\s+REPLACE\s+)?(?:MATERIALIZED\s+)?VIEW\s+(?:IF\s+NOT\s+EXISTS\s+)?([\w".]+)`)
	sqlFuncRe  = regexp.MustCompile(`(?i)\bCREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)\s+([\w".]+)`)
	sqlFKRe    = regexp.MustCompile(`(?i)\bREFERENCES\s+([\w".]+)`)
	sqlFromRe  = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([\w".]+)`)
)

// sqlStatement is one CREATE statement's extent within the file.
type sqlStatement struct {
	start  int // byte offset of the CREATE keyword
	end    int // exclusive; next CREATE or EOF
	nodeID string
	isView bool
}

func (s *sqlExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	fn := fileNode("sql", file)
	res.Nodes = append(res.Nodes, fn)

	text := sqlStripComments(string(src))

	type match struct {
		offset int
		name   string
		kind   model.Kind
		isView bool
	}
	var matches []match
	collect := func(re *regexp.Regexp, kind model.Kind, isView bool) {
		for _, m := range re.FindAllStringSubmatchIndex(text, -1) {
			matches = append(matches, match{
				offset: m[0],
				name:   sqlIdent(text[m[2]:m[3]]),
				kind:   kind,
				isView: isView,
			})
		}
	}
	collect(sqlTableRe, model.KindStruct, false)
	collect(sqlViewRe, model.KindType, true)
	collect(sqlFuncRe, model.KindFunction, false)
	sort.Slice(matches, func(i, j int) bool { return matches[i].offset < matches[j].offset })

	var statements []sqlStatement
	seen := map[string]bool{}
	for i, m := range matches {
		if m.name == "" {
			continue
		}
		end := len(text)
		if i+1 < len(matches) {
			end = matches[i+1].offset
		}
		id := model.NodeID("sql", m.kind, file.Path, m.name)
		if !seen[id] {
			seen[id] = true
			line := sqlLineAt(text, m.offset)
			n := model.Node{
				ID:        id,
				Label:     m.name,
				Kind:      m.kind,
				Language:  "sql",
				File:      file.Path,
				Span:      model.Span{StartLine: line},
				Signature: sqlSignature(text, m.offset),
			}
			res.Nodes = append(res.Nodes, n)
			res.Edges = append(res.Edges, containsEdge(fn.ID, id, file.Path, line))
		}
		statements = append(statements, sqlStatement{start: m.offset, end: end, nodeID: id, isView: m.isView})
	}

	// References, attributed to their enclosing statement: foreign keys from
	// any statement, FROM/JOIN sources from views only (tables' column
	// definitions don't contain source relations).
	seenRefs := map[string]bool{}
	emit := func(stmt sqlStatement, re *regexp.Regexp) {
		body := text[stmt.start:stmt.end]
		for _, m := range re.FindAllStringSubmatchIndex(body, -1) {
			name := sqlIdent(body[m[2]:m[3]])
			if name == "" {
				continue
			}
			key := stmt.nodeID + "\x00" + name
			if seenRefs[key] {
				continue
			}
			seenRefs[key] = true
			res.Refs = append(res.Refs, model.Ref{
				FromID:   stmt.nodeID,
				Name:     name,
				Relation: model.RelReferences,
				Line:     sqlLineAt(text, stmt.start+m[0]),
			})
		}
	}
	for _, stmt := range statements {
		emit(stmt, sqlFKRe)
		if stmt.isView {
			emit(stmt, sqlFromRe)
		}
	}
	return res, nil
}

// sqlStripComments blanks out -- line comments and /* */ blocks while
// preserving byte offsets and newlines, so positions keep pointing at source.
func sqlStripComments(text string) string {
	out := []byte(text)
	for i := 0; i < len(out); i++ {
		switch {
		case out[i] == '-' && i+1 < len(out) && out[i+1] == '-':
			for ; i < len(out) && out[i] != '\n'; i++ {
				out[i] = ' '
			}
		case out[i] == '/' && i+1 < len(out) && out[i+1] == '*':
			for ; i < len(out); i++ {
				if out[i] == '\n' {
					continue
				}
				if out[i] == '*' && i+1 < len(out) && out[i+1] == '/' {
					out[i], out[i+1] = ' ', ' '
					i++
					break
				}
				out[i] = ' '
			}
		case out[i] == '\'':
			// Blank string literals too — they can contain keyword lookalikes.
			for i++; i < len(out) && out[i] != '\''; i++ {
				if out[i] != '\n' {
					out[i] = ' '
				}
			}
		}
	}
	return string(out)
}

// sqlIdent normalizes an identifier: quotes stripped, lowercased (SQL
// identifiers are case-insensitive in every mainstream dialect's default).
func sqlIdent(raw string) string {
	name := strings.Trim(raw, `"`)
	name = strings.TrimSuffix(name, ".")
	if name == "" || name == "." {
		return ""
	}
	return strings.ToLower(name)
}

func sqlLineAt(text string, offset int) int {
	return strings.Count(text[:offset], "\n") + 1
}

// sqlSignature returns the CREATE statement's first line.
func sqlSignature(text string, offset int) string {
	line := text[offset:]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "("))
}
