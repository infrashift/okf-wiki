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
// doc nodes, top-level headings become heading nodes, and vanilla markdown
// links become `references` refs that resolution can attach to ANY extracted
// repo file — docs linking code included.
type markdownExtractor struct{}

func (m *markdownExtractor) Language() string { return "markdown" }
func (m *markdownExtractor) Close()           {}

func (m *markdownExtractor) Extract(file detect.File, src []byte) (*Result, error) {
	res := &Result{}
	doc := fileNode("markdown", file)
	doc.ID = model.NodeID("markdown", model.KindDoc, file.Path, "")
	doc.Kind = model.KindDoc
	res.Nodes = append(res.Nodes, doc)

	lines := strings.Split(string(src), "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		level, text := headingOf(trimmed)
		if level == 0 {
			continue
		}
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
		// Repeated headings produce identical ids; keep the first.
		if !containsNode(res.Nodes, h.ID) {
			res.Nodes = append(res.Nodes, h)
			res.Edges = append(res.Edges, containsEdge(doc.ID, h.ID, file.Path, h.Span.StartLine))
		}
	}

	fromDir := ""
	if i := strings.LastIndex(file.Path, "/"); i >= 0 {
		fromDir = file.Path[:i]
	}
	seen := map[string]bool{}
	for _, m := range mdLinkRe.FindAllStringSubmatch(string(src), -1) {
		href := m[1]
		if i := strings.IndexByte(href, '#'); i >= 0 {
			href = href[:i]
		}
		if href == "" || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
			continue
		}
		// NormalizeTarget handles ./, ../ and bundle-absolute paths; here the
		// "bundle" is the repo, so targets resolve repo-relative.
		target := graph.NormalizeTarget(href, fromDir)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		res.Refs = append(res.Refs, model.Ref{
			FromID:   doc.ID,
			Name:     target,
			Relation: model.RelReferences,
		})
	}
	return res, nil
}

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
