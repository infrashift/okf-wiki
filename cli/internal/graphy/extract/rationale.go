package extract

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"okfw/cli/internal/graph"
	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

// Rationale extraction records *why* code is the way it is: marked comments
// (WHY:, DECISION:, TODO: …) and architecture decision records. It runs as a
// post-pass over any extractor's Result, so it covers every supported
// language — including the pure-Go config extractors, which is where "why is
// this odd value here" most often lives and where a tree-sitter comment query
// could not reach.

// Default marker and ADR-directory sets, also used for the CLI flag defaults.
var (
	DefaultMarkers = []string{"WHY", "RATIONALE", "DECISION", "HACK", "FIXME", "TODO"}
	DefaultADRDirs = []string{"adr", "adrs", "decisions"}
)

// Caps keep one pathological file from flooding the graph. They mirror the
// config extractor's maxConfigEntries in spirit.
const (
	maxRationaleLines   = 20
	maxRationaleChars   = 2000
	maxRationalePerFile = 64
	maxADRRefs          = 40
	maxADRBodyChars     = 4000
	// maxOwnerGap is how many blank/decorative lines may sit between a comment
	// and the definition it explains before the link is considered too loose.
	maxOwnerGap = 3
)

// RationaleOptions configures rationale extraction.
type RationaleOptions struct {
	Enabled bool
	Markers []string // uppercase, colon-less
	ADRDirs []string // path segments identifying ADR directories
}

// DefaultRationaleOptions returns the enabled-by-default configuration.
func DefaultRationaleOptions() RationaleOptions {
	return RationaleOptions{Enabled: true, Markers: DefaultMarkers, ADRDirs: DefaultADRDirs}
}

// markerSet normalizes the configured markers for lookup.
func (o RationaleOptions) markerSet() map[string]bool {
	markers := o.Markers
	if len(markers) == 0 {
		markers = DefaultMarkers
	}
	set := make(map[string]bool, len(markers))
	for _, m := range markers {
		if m = strings.ToUpper(strings.TrimSpace(m)); m != "" {
			set[strings.TrimSuffix(m, ":")] = true
		}
	}
	return set
}

func (o RationaleOptions) adrDirs() []string {
	if len(o.ADRDirs) == 0 {
		return DefaultADRDirs
	}
	return o.ADRDirs
}

// attachRationale appends rationale nodes, edges and refs to an
// already-complete Result. It only ever APPENDS, which is what preserves the
// package-wide invariant that Nodes[0] is the file node.
func attachRationale(file detect.File, src []byte, res *Result, opts RationaleOptions) {
	if res == nil || len(res.Nodes) == 0 {
		return
	}
	if file.Language == "markdown" {
		// Markdown gets ADR handling instead of marker scanning: an HTML
		// comment would otherwise collide with the okfw export's own
		// `<!-- graphy:node … -->` body marker and round-trip exported
		// concepts back into the graph as rationale.
		attachADR(file, src, res, opts)
		return
	}
	attachMarkers(file, src, res, opts)
}

// attachMarkers scans source comments for markers and links each to the entity
// it explains.
func attachMarkers(file detect.File, src []byte, res *Result, opts RationaleOptions) {
	markers := opts.markerSet()
	fileNodeID := res.Nodes[0].ID
	candidates := ownerCandidates(res.Nodes)

	ordinals := map[string]int{} // ownerQual\x00MARKER → next ordinal
	emitted := 0

	for _, block := range scanComments(file.Language, src) {
		for _, r := range block.rationales(markers) {
			if emitted >= maxRationalePerFile {
				return
			}
			ownerID := fileNodeID
			if idx := ownerFor(candidates, r.startLine, r.endLine); idx >= 0 {
				ownerID = candidates[idx].ID
			}
			ownerQual := qualOf(ownerID)

			key := ownerQual + "\x00" + r.marker
			ord := ordinals[key]
			ordinals[key]++

			id := model.NodeID(file.Language, model.KindRationale, file.Path,
				model.RationaleQual(ownerQual, r.marker, ord))
			res.Nodes = append(res.Nodes, model.Node{
				ID:       id,
				Label:    r.marker + ": " + r.summary,
				Kind:     model.KindRationale,
				Language: file.Language,
				File:     file.Path,
				Span:     model.Span{StartLine: r.startLine, EndLine: r.endLine},
				Doc:      r.summary,
				Body:     r.body,
			})
			res.Edges = append(res.Edges,
				containsEdge(fileNodeID, id, file.Path, r.startLine),
				model.Edge{
					Source: id, Target: ownerID,
					Relation:   model.RelExplains,
					Confidence: model.ConfExtracted,
					File:       file.Path, Line: r.startLine,
				})
			emitted++
		}
	}
}

// ownerCandidates returns the definition nodes a rationale may attach to,
// excluding the file node itself (which is the fallback) and nodes that carry
// no useful span or are not real definitions.
func ownerCandidates(nodes []model.Node) []model.Node {
	var out []model.Node
	for _, n := range nodes[1:] {
		switch n.Kind {
		case model.KindHeading, model.KindDoc, model.KindExternal, model.KindRationale:
			continue
		}
		if n.Span.StartLine > 0 {
			out = append(out, n)
		}
	}
	return out
}

// ownerFor picks the entity a comment block explains:
//
//  1. the next definition below it, within maxOwnerGap lines — the "comment
//     sits above the thing it explains" convention, which dominates in practice;
//  2. otherwise the smallest definition whose span encloses the comment, which
//     handles a marker written inside a function body;
//  3. otherwise -1, meaning the file node.
//
// Ties break on node id so the result never depends on extractor ordering.
func ownerFor(candidates []model.Node, startLine, endLine int) int {
	best := -1
	for i, n := range candidates {
		gap := n.Span.StartLine - endLine
		if gap < 0 || gap > maxOwnerGap {
			continue
		}
		if best < 0 || betterBelow(n, candidates[best], endLine) {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	for i, n := range candidates {
		// Several pure-Go extractors leave EndLine unset; without it a node
		// cannot be said to enclose anything.
		if n.Span.EndLine == 0 {
			continue
		}
		if n.Span.StartLine > startLine || n.Span.EndLine < startLine {
			continue
		}
		if best < 0 || smallerSpan(n, candidates[best]) {
			best = i
		}
	}
	return best
}

func betterBelow(a, b model.Node, endLine int) bool {
	da, db := a.Span.StartLine-endLine, b.Span.StartLine-endLine
	if da != db {
		return da < db
	}
	return a.ID < b.ID
}

func smallerSpan(a, b model.Node) bool {
	sa := a.Span.EndLine - a.Span.StartLine
	sb := b.Span.EndLine - b.Span.StartLine
	if sa != sb {
		return sa < sb
	}
	return a.ID < b.ID
}

// qualOf returns the qualified-name half of a node id. Node ids are
// "<lang>:<kind>:<file>#<qual>", and the qual may itself be empty (file nodes).
func qualOf(nodeID string) string {
	if i := strings.LastIndex(nodeID, "#"); i >= 0 {
		return nodeID[i+1:]
	}
	return ""
}

// rationale is one marked comment ready to become a node.
type rationale struct {
	marker             string
	summary            string // first line, for Doc/Label
	body               string // full text, possibly multi-line
	startLine, endLine int
}

// rationales splits a comment block into the marked rationales it contains.
//
// Every line is examined, not just the block's first: the overwhelmingly
// common shape is a doc comment whose WHY sits below the summary line
// ("// Helper does a thing." / "// WHY: because X"). A marker must still be
// the *leading* word of its line, so prose mentions like "see TODO: later"
// never trigger. Each marker's text runs until the next marker or the end of
// the block.
func (b commentBlock) rationales(markers map[string]bool) []rationale {
	var out []rationale
	for i, ln := range b.lines {
		marker, rest, ok := markerOf(ln.text, markers)
		if !ok {
			continue
		}
		parts := []string{}
		if rest != "" {
			parts = append(parts, rest)
		}
		end := ln.line
		for j := i + 1; j < len(b.lines) && len(parts) < maxRationaleLines; j++ {
			if _, _, isMarker := markerOf(b.lines[j].text, markers); isMarker {
				break
			}
			if b.lines[j].text == "" && len(parts) == 0 {
				continue
			}
			parts = append(parts, b.lines[j].text)
			end = b.lines[j].line
		}
		body := strings.TrimSpace(strings.Join(parts, "\n"))
		if body == "" {
			continue // a bare "TODO:" carries no reasoning
		}
		if len(body) > maxRationaleChars {
			body = body[:maxRationaleChars]
		}
		summary := body
		if i := strings.IndexByte(summary, '\n'); i >= 0 {
			summary = summary[:i]
		}
		out = append(out, rationale{
			marker: marker, summary: summary, body: body,
			startLine: ln.line, endLine: end,
		})
	}
	return out
}

// markerOf reports whether a comment line opens with "<MARKER>:", returning
// the marker and the remaining text. Requiring the marker to be the leading
// word is what keeps "http://…" and "see TODO: later" from matching.
func markerOf(text string, markers map[string]bool) (marker, rest string, ok bool) {
	i := strings.IndexByte(text, ':')
	if i <= 0 {
		return "", "", false
	}
	word := strings.ToUpper(strings.TrimSpace(text[:i]))
	if !markers[word] {
		return "", "", false
	}
	return word, strings.TrimSpace(text[i+1:]), true
}

// ---------------------------------------------------------------------------
// Comment scanning

// stringDelim describes one string-literal form.
type stringDelim struct {
	open, close string
	escape      byte // 0 = no escape processing (raw strings)
	multiline   bool
}

// commentSyntax describes how a language delimits comments and string
// literals. Tracking literals is the point: the realistic false positive is a
// marker inside a string ("TODO: fix" as data), and only literal awareness
// rules it out. This is the same technique sqlStripComments already uses.
type commentSyntax struct {
	line       []string
	blockOpen  string
	blockClose string
	strings    []stringDelim
}

var (
	quoted   = stringDelim{open: `"`, close: `"`, escape: '\\'}
	sQuoted  = stringDelim{open: `'`, close: `'`, escape: '\\'}
	backtick = stringDelim{open: "`", close: "`", multiline: true}
	tripleD  = stringDelim{open: `"""`, close: `"""`, escape: '\\', multiline: true}
	tripleS  = stringDelim{open: `'''`, close: `'''`, escape: '\\', multiline: true}
	// SQL escapes a quote by doubling it, not with a backslash; leaving escape
	// unset means '' simply closes then reopens, which lands in the same place.
	sqlQuoted = stringDelim{open: `'`, close: `'`}
)

var syntaxByLang = map[string]commentSyntax{
	"go": {line: []string{"//"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{backtick, quoted, {open: `'`, close: `'`, escape: '\\'}}},
	"javascript": {line: []string{"//"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{backtick, quoted, sQuoted}},
	"typescript": {line: []string{"//"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{backtick, quoted, sQuoted}},
	"python": {line: []string{"#"},
		strings: []stringDelim{tripleD, tripleS, quoted, sQuoted}},
	"yaml": {line: []string{"#"}, strings: []stringDelim{quoted, sQuoted}},
	"json": {line: []string{"//"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{quoted}},
	"sql": {line: []string{"--"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{sqlQuoted, quoted}},
	"terraform": {line: []string{"#", "//"}, blockOpen: "/*", blockClose: "*/",
		strings: []stringDelim{quoted}},
	"cue": {line: []string{"//"},
		strings: []stringDelim{tripleD, quoted, sQuoted}},
}

// commentLine is one physical comment line, with its marker prefix stripped.
type commentLine struct {
	line int // 1-based
	text string
}

// commentBlock is a run of contiguous comment lines.
type commentBlock struct {
	startLine, endLine int
	lines              []commentLine
}

// scanComments extracts every comment in the source, grouped into contiguous
// blocks, skipping anything inside a string literal. Languages with no
// registered syntax yield nothing rather than guessing.
func scanComments(lang string, src []byte) []commentBlock {
	syn, ok := syntaxByLang[lang]
	if !ok {
		return nil
	}
	var lines []commentLine
	line, i, n := 1, 0, len(src)

	for i < n {
		if src[i] == '\n' {
			line++
			i++
			continue
		}
		if d, w := syn.stringAt(src, i); w > 0 {
			i, line = skipString(src, i, d, line)
			continue
		}
		if syn.blockOpen != "" && hasPrefixAt(src, i, syn.blockOpen) {
			start := line
			i += len(syn.blockOpen)
			from := i
			for i < n && !hasPrefixAt(src, i, syn.blockClose) {
				if src[i] == '\n' {
					line++
				}
				i++
			}
			text := string(src[from:i])
			if i < n {
				i += len(syn.blockClose)
			}
			for k, t := range strings.Split(text, "\n") {
				lines = append(lines, commentLine{line: start + k, text: stripBlockPrefix(t)})
			}
			continue
		}
		if p := syn.lineCommentAt(src, i); p != "" {
			i += len(p)
			from := i
			for i < n && src[i] != '\n' {
				i++
			}
			lines = append(lines, commentLine{line: line, text: strings.TrimSpace(string(src[from:i]))})
			continue
		}
		i++
	}
	return groupBlocks(lines)
}

// groupBlocks joins comment lines that sit on consecutive source lines.
func groupBlocks(lines []commentLine) []commentBlock {
	var out []commentBlock
	for _, ln := range lines {
		if n := len(out); n > 0 && out[n-1].endLine+1 == ln.line {
			out[n-1].lines = append(out[n-1].lines, ln)
			out[n-1].endLine = ln.line
			continue
		}
		out = append(out, commentBlock{startLine: ln.line, endLine: ln.line, lines: []commentLine{ln}})
	}
	return out
}

// stringAt reports the string delimiter opening at src[i], if any. Delimiters
// are tried in registration order so longer forms (""" before ") win.
func (s commentSyntax) stringAt(src []byte, i int) (stringDelim, int) {
	for _, d := range s.strings {
		if hasPrefixAt(src, i, d.open) {
			return d, len(d.open)
		}
	}
	return stringDelim{}, 0
}

// lineCommentAt reports the line-comment prefix starting at src[i], if any.
func (s commentSyntax) lineCommentAt(src []byte, i int) string {
	for _, p := range s.line {
		if hasPrefixAt(src, i, p) {
			return p
		}
	}
	return ""
}

// skipString advances past a string literal, returning the new offset and line.
func skipString(src []byte, i int, d stringDelim, line int) (int, int) {
	n := len(src)
	i += len(d.open)
	for i < n {
		if d.escape != 0 && src[i] == d.escape && i+1 < n {
			if src[i+1] == '\n' {
				line++
			}
			i += 2
			continue
		}
		if hasPrefixAt(src, i, d.close) {
			return i + len(d.close), line
		}
		if src[i] == '\n' {
			line++
			if !d.multiline {
				// An unterminated single-line literal: resync at the newline
				// rather than swallowing the rest of the file.
				return i + 1, line
			}
		}
		i++
	}
	return n, line
}

func hasPrefixAt(src []byte, i int, prefix string) bool {
	return i+len(prefix) <= len(src) && string(src[i:i+len(prefix)]) == prefix
}

// stripBlockPrefix removes the decorative leading '*' of a block-comment
// continuation line.
func stripBlockPrefix(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "*") && !strings.HasPrefix(s, "*/") {
		s = strings.TrimSpace(s[1:])
	}
	return s
}

// ---------------------------------------------------------------------------
// Architecture decision records

var (
	adrFileRe   = regexp.MustCompile(`(?i)^(adr[-_]?)?\d{3,4}[-_].+\.md$`)
	adrStatusRe = regexp.MustCompile(`(?im)^status:\s*"?([a-z]+)"?\s*$`)
	adrIdentRe  = regexp.MustCompile("`([^`\n]+)`")
	identRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
)

// adrStatuses is the ADR status vocabulary. It is deliberately disjoint from
// okfw's todo statuses (open/in_progress/closed) so a concept in a bundle is
// never mistaken for a decision record.
var adrStatuses = map[string]bool{
	"proposed": true, "accepted": true, "rejected": true,
	"superseded": true, "deprecated": true,
}

// adrStoplist keeps common English words in backticks from being treated as
// symbol references.
var adrStoplist = map[string]bool{
	"the": true, "and": true, "for": true, "not": true, "yes": true,
	"true": true, "false": true, "null": true, "nil": true, "this": true,
	"that": true, "with": true, "from": true, "use": true, "using": true,
}

// isADR reports whether a markdown file is an architecture decision record.
func isADR(relPath string, src []byte, adrDirs []string) bool {
	segs := strings.Split(relPath, "/")
	for _, s := range segs {
		// Bundle concepts are markdown and ARE listed by detect.List, so
		// without this an exported concept would re-enter as an ADR.
		if s == ".okfw" || s == ".graphy" {
			return false
		}
	}
	want := map[string]bool{}
	for _, d := range adrDirs {
		want[strings.ToLower(strings.TrimSpace(d))] = true
	}
	for _, s := range segs[:max(0, len(segs)-1)] {
		if want[strings.ToLower(s)] {
			return true
		}
	}
	if adrFileRe.MatchString(segs[len(segs)-1]) {
		return true
	}
	if fm, ok := frontmatterBlock(src); ok {
		if m := adrStatusRe.FindStringSubmatch(fm); m != nil && adrStatuses[strings.ToLower(m[1])] {
			return true
		}
	}
	return false
}

// frontmatterBlock returns the raw YAML frontmatter, if the document has any.
// This is a local reader rather than config.ParseFrontmatter on purpose:
// extraction has no business depending on okfw's concept schema.
func frontmatterBlock(src []byte) (string, bool) {
	s := string(src)
	if !strings.HasPrefix(s, "---\n") {
		return "", false
	}
	end := strings.Index(s[4:], "\n---")
	if end < 0 {
		return "", false
	}
	return s[4 : 4+end+1], true
}

// attachADR turns a decision record into a single rationale node linked to the
// entities it names. One node per file: an ADR is one decision by
// construction, and per-section nodes would just duplicate the heading nodes
// the markdown extractor already emits.
func attachADR(file detect.File, src []byte, res *Result, opts RationaleOptions) {
	if !isADR(file.Path, src, opts.adrDirs()) {
		return
	}
	docID := res.Nodes[0].ID
	title, summary, body := adrContent(src, file.Path)

	id := model.NodeID(file.Language, model.KindRationale, file.Path,
		model.RationaleQual("", "ADR", 0))
	res.Nodes = append(res.Nodes, model.Node{
		ID:       id,
		Label:    "ADR: " + title,
		Kind:     model.KindRationale,
		Language: file.Language,
		File:     file.Path,
		Span:     model.Span{StartLine: 1},
		Doc:      summary,
		Body:     body,
	})
	res.Edges = append(res.Edges, containsEdge(docID, id, file.Path, 1))

	// Named entities become refs, resolved against the symbol index in the
	// build's second pass — so they inherit the existing confidence ladder.
	fromDir := ""
	if i := strings.LastIndex(file.Path, "/"); i >= 0 {
		fromDir = file.Path[:i]
	}
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] || len(seen) >= maxADRRefs {
			return
		}
		seen[name] = true
		res.Refs = append(res.Refs, model.Ref{FromID: id, Name: name, Relation: model.RelExplains})
	}
	for _, m := range adrIdentRe.FindAllStringSubmatch(string(src), -1) {
		name := strings.TrimSpace(m[1])
		if len(name) < 3 || !identRe.MatchString(name) || adrStoplist[strings.ToLower(name)] {
			continue
		}
		add(name)
	}
	for _, m := range mdLinkRe.FindAllStringSubmatch(string(src), -1) {
		href := m[1]
		if i := strings.IndexByte(href, '#'); i >= 0 {
			href = href[:i]
		}
		if href == "" || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
			continue
		}
		add(graph.NormalizeTarget(href, fromDir))
	}

	// The markdown extractor already turned these same names into heading-owned
	// `references` refs. For an ADR that is strictly worse information: an
	// `explains` edge says the document justifies the entity, which is what
	// parents the exported concept and what graphy_rationale filters on, while a
	// duplicate `references` edge only doubles the graph's edge count and the
	// bundle's bullet count. Drop the duplicates and keep the stronger relation.
	res.Refs = slices.DeleteFunc(res.Refs, func(r model.Ref) bool {
		if r.Relation != model.RelReferences {
			return false
		}
		name, _ := strings.CutSuffix(r.Name, "()")
		return seen[name] || seen[r.Name]
	})
}

// adrContent pulls the title, one-line summary and body out of a decision
// record.
func adrContent(src []byte, relPath string) (title, summary, body string) {
	text := string(src)
	if fm, ok := frontmatterBlock(src); ok {
		title = firstYAMLScalar(fm, "title")
		summary = firstYAMLScalar(fm, "description")
		text = text[strings.Index(text[4:], "\n---")+4:]
	}
	lines := strings.Split(text, "\n")
	sections := map[string][]string{}
	current := ""
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "# ") && title == "" {
			title = strings.TrimSpace(t[2:])
			continue
		}
		if strings.HasPrefix(t, "## ") {
			current = strings.ToLower(strings.TrimSpace(t[3:]))
			continue
		}
		if current != "" {
			sections[current] = append(sections[current], ln)
		}
	}
	if title == "" {
		title = strings.TrimSuffix(relPath[strings.LastIndex(relPath, "/")+1:], ".md")
	}
	if summary == "" {
		summary = firstParagraph(sections["decision"])
	}
	if summary == "" {
		for _, ln := range lines {
			if t := strings.TrimSpace(ln); t != "" && !strings.HasPrefix(t, "#") {
				summary = t
				break
			}
		}
	}

	var b strings.Builder
	for _, name := range []string{"decision", "context"} {
		if p := firstParagraph(sections[name]); p != "" {
			fmt.Fprintf(&b, "**%s:** %s\n\n", strings.ToUpper(name[:1])+name[1:], p)
		}
	}
	body = strings.TrimSpace(b.String())
	if body == "" {
		body = summary
	}
	if len(body) > maxADRBodyChars {
		body = body[:maxADRBodyChars]
	}
	return title, summary, body
}

// firstParagraph joins the first non-empty run of lines in a section.
func firstParagraph(lines []string) string {
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" {
			if len(out) > 0 {
				break
			}
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, " ")
}

// firstYAMLScalar reads a top-level `key: value` out of a frontmatter block.
func firstYAMLScalar(fm, key string) string {
	for _, ln := range strings.Split(fm, "\n") {
		if !strings.HasPrefix(ln, key+":") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(ln, key+":"))
		return strings.Trim(v, `"'`)
	}
	return ""
}
