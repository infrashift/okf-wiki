package extract

import (
	"regexp"
	"strings"

	"okfw/cli/internal/graph"
	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func init() {
	register("markdown", func() (Extractor, error) { return &markdownExtractor{}, nil })
}

// markdownExtractor is pure Go: markdown needs no grammar. Documents become
// doc nodes, top-level headings become heading nodes, and both vanilla markdown
// links and backticked identifiers in prose become `references` refs that
// resolution can attach to ANY extracted repo file or symbol — docs linking code
// included.
//
// Backticked identifiers are what make documentation reachable from code at all.
// A link only resolves when a doc happens to name a *path*, which most prose
// does not: across this repository's 38 documents, links produced exactly one
// reference edge, and it pointed at another document. Prose overwhelmingly names
// things the way this sentence would — `ToOkfw`, `resolveCandidates` — so that is
// what has to be read.
//
// Refs are attributed to the enclosing H1/H2 rather than to the document, so a
// section links to what it describes and an agent reading one section is not
// handed the whole file's worth of targets.
type markdownExtractor struct{}

func (m *markdownExtractor) Language() string { return "markdown" }
func (m *markdownExtractor) Close()           {}

// maxDocRefs caps references per document. Prose is unbounded and a reference
// list is only useful while it is short enough to read.
const maxDocRefs = 60

func (m *markdownExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	doc := fileNode("markdown", file)
	doc.ID = model.NodeID("markdown", model.KindDoc, file.Path, "")
	doc.Kind = model.KindDoc
	res.Nodes = append(res.Nodes, doc)

	fromDir := ""
	if i := strings.LastIndex(file.Path, "/"); i >= 0 {
		fromDir = file.Path[:i]
	}
	// Keyed by owner+name, so the same symbol named under two sections links
	// from both — that is the section-level precision the attribution is for.
	seen := map[string]bool{}
	addRef := func(fromID, name string, line int) {
		key := fromID + "\x00" + name
		if name == "" || seen[key] || len(seen) >= maxDocRefs {
			return
		}
		seen[key] = true
		res.Refs = append(res.Refs, model.Ref{
			FromID: fromID, Name: name, Relation: model.RelReferences, Line: line,
		})
	}

	lines := strings.Split(string(src), "\n")
	inFence := false
	owner := doc.ID // the H1/H2 currently in scope
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if level, text := headingOf(trimmed); level > 0 {
			if doc.Doc == "" && level == 1 {
				res.Nodes[0].Doc = text // document title
			}
			if level > 2 {
				continue // only H1/H2 become nodes — deeper levels are noise
			}
			h := model.Node{
				ID:       model.NodeID("markdown", model.KindHeading, file.Path, anchorOf(text)),
				Label:    text,
				Kind:     model.KindHeading,
				Language: "markdown",
				File:     file.Path,
				Span:     model.Span{StartLine: i + 1},
			}
			// Repeated headings produce identical ids; keep the first, and let
			// the second's content hang off it rather than off the document.
			if !containsNode(res.Nodes, h.ID) {
				res.Nodes = append(res.Nodes, h)
				res.Edges = append(res.Edges, containsEdge(doc.ID, h.ID, file.Path, h.Span.StartLine))
			}
			owner = h.ID
			continue
		}

		for _, m := range mdLinkRe.FindAllStringSubmatch(line, -1) {
			if target := linkTarget(m[1], fromDir); target != "" {
				addRef(owner, target, i+1)
			}
		}
		for _, m := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
			if name := docIdentifier(m[1]); name != "" {
				addRef(owner, name, i+1)
			}
		}
	}
	return res, nil
}

// linkTarget normalizes a markdown href to a repo-relative path, keeping a
// line anchor so resolution can aim at the symbol on that line rather than at
// the whole file. Returns "" for external links and unresolvable targets.
func linkTarget(href, fromDir string) string {
	anchor := ""
	if i := strings.IndexByte(href, '#'); i >= 0 {
		if lineAnchorRe.MatchString(href[i:]) {
			anchor = href[i:]
		}
		href = href[:i]
	}
	if href == "" || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
		return ""
	}
	// NormalizeTarget handles ./, ../ and bundle-absolute paths; here the
	// "bundle" is the repo, so targets resolve repo-relative.
	target := graph.NormalizeTarget(href, fromDir)
	if target == "" {
		return ""
	}
	return target + anchor
}

// docIdentifier reports whether a span of inline code names a code entity,
// returning it. Prose backticks hold plenty that is not an identifier — flags,
// shell commands, file paths, English words — and only identifier-shaped,
// non-stoplisted spans are worth resolving.
func docIdentifier(s string) string {
	// A trailing "()" is how prose writes a call; the symbol index has neither,
	// and it has to come off before the identifier shape is checked.
	s = strings.TrimSuffix(strings.TrimSpace(s), "()")
	if len(s) < 3 || !identRe.MatchString(s) || adrStoplist[strings.ToLower(s)] {
		return ""
	}
	return s
}

// lineAnchorRe matches a GitHub-style line anchor ("#L42").
var lineAnchorRe = regexp.MustCompile(`^#L\d+$`)

// inlineCodeRe matches a single-backtick span within one line.
var inlineCodeRe = regexp.MustCompile("`([^`]+)`")

// mdLinkRe matches vanilla markdown links to any target — unlike okfw's
// graph.ExtractLinkTargets, which is .md-only, docs here may link code files.
var mdLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)

func headingOf(line string) (int, string) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level >= len(line) || line[level] != ' ' {
		return 0, ""
	}
	return level, strings.TrimSpace(line[level+1:])
}

func anchorOf(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func containsNode(nodes []model.Node, id string) bool {
	for i := range nodes {
		if nodes[i].ID == id {
			return true
		}
	}
	return false
}
