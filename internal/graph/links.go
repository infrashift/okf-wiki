package graph

import (
	"sort"
	"strings"
)

// linkDest is the destination of one internal markdown link: the byte range of
// its path within the body (the "#fragment" excluded) and whether it was
// written in angle brackets.
type linkDest struct {
	start, end int
	angled     bool
}

// linkDests finds the destination of every markdown link to a .md file in body:
// inline links ([text](dest "title"), including images and links whose text
// wraps across lines) and reference definitions ([label]: dest).
//
// It is a scanner rather than a regular expression because the regex it
// replaced got the grammar wrong both ways. It rewrote "links" inside fenced
// code and inline code during a move — silently editing example prose — and it
// missed titled links, angle-bracketed destinations, reference definitions and
// percent-encoded paths, which therefore broke on every move and were invisible
// to backlinks, the graph and delete's inbound report.
//
// Indented code blocks are not recognized: telling one from an indented list
// continuation needs the full block parser, and a false negative there costs
// an unrewritten example, where a false positive would cost a lost link.
func linkDests(body string) []linkDest {
	fences := fencedRanges(body)
	var out []linkDest
	add := func(d linkDest, ok bool) {
		if ok && isInternalMarkdown(body[d.start:d.end]) {
			out = append(out, d)
		}
	}

	// Reference definitions are line-level constructs.
	for _, ln := range lines(body) {
		if inRanges(fences, ln[0]) {
			continue
		}
		add(refDefinition(body, ln[0], ln[1]))
	}

	fi := 0
	for i := 0; i < len(body); {
		for fi < len(fences) && fences[fi][1] <= i {
			fi++
		}
		if fi < len(fences) && i >= fences[fi][0] {
			i = fences[fi][1]
			continue
		}
		switch body[i] {
		case '\\':
			i += 2
		case '`':
			i = skipCodeSpan(body, i)
		case '[':
			if d, next, ok := inlineLink(body, i, fences); ok {
				add(d, true)
				i = next
			} else {
				i++
			}
		default:
			i++
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].start < out[b].start })
	return out
}

// lines returns the [start, end) offsets of each line, without its newline.
func lines(body string) [][2]int {
	var out [][2]int
	for off := 0; off < len(body); {
		end := strings.IndexByte(body[off:], '\n')
		if end < 0 {
			out = append(out, [2]int{off, len(body)})
			break
		}
		out = append(out, [2]int{off, off + end})
		off += end + 1
	}
	return out
}

// fencedRanges returns the byte ranges of fenced code blocks (``` or ~~~),
// sorted. An unclosed fence runs to the end of the body, as in CommonMark.
func fencedRanges(body string) [][2]int {
	var out [][2]int
	open := -1
	var char byte
	var n int
	for _, ln := range lines(body) {
		line := strings.TrimRight(body[ln[0]:ln[1]], "\r")
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) > 3 || trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
			continue
		}
		c := trimmed[0]
		k := 0
		for k < len(trimmed) && trimmed[k] == c {
			k++
		}
		if k < 3 {
			continue
		}
		if open < 0 {
			if c == '`' && strings.ContainsRune(trimmed[k:], '`') {
				continue // a backtick fence's info string may not contain one
			}
			open, char, n = ln[0], c, k
			continue
		}
		if c == char && k >= n && strings.TrimSpace(trimmed[k:]) == "" {
			out = append(out, [2]int{open, ln[1]})
			open = -1
		}
	}
	if open >= 0 {
		out = append(out, [2]int{open, len(body)})
	}
	return out
}

func inRanges(rs [][2]int, i int) bool {
	for _, r := range rs {
		if i >= r[0] && i < r[1] {
			return true
		}
	}
	return false
}

// skipCodeSpan returns the offset just past the inline code span opening at i,
// or past the backtick run itself when no run of the same length closes it.
func skipCodeSpan(body string, i int) int {
	k := i
	for k < len(body) && body[k] == '`' {
		k++
	}
	run := k - i
	for j := k; j < len(body); {
		if body[j] != '`' {
			j++
			continue
		}
		e := j
		for e < len(body) && body[e] == '`' {
			e++
		}
		if e-j == run {
			return e
		}
		j = e
	}
	return k
}

// inlineLink parses "[text](dest ...)" with the bracket at i. It returns the
// destination, the offset after the closing parenthesis, and whether a link was
// found.
func inlineLink(body string, i int, fences [][2]int) (linkDest, int, bool) {
	// Link text: balanced brackets, escapes honoured, code spans opaque, and no
	// blank line or code block inside (either ends the paragraph).
	depth := 0
	j := i
	for ; j < len(body); j++ {
		switch body[j] {
		case '\\':
			j++
			continue
		case '`':
			j = skipCodeSpan(body, j) - 1
			continue
		case '[':
			depth++
		case ']':
			depth--
		case '\n':
			if strings.HasPrefix(strings.TrimLeft(body[j+1:], " \t\r"), "\n") || inRanges(fences, j+1) {
				return linkDest{}, 0, false
			}
		}
		if depth == 0 {
			break
		}
	}
	if j >= len(body) || j+1 >= len(body) || body[j+1] != '(' {
		return linkDest{}, 0, false
	}

	k := skipSpace(body, j+2)
	var d linkDest
	if k < len(body) && body[k] == '<' {
		end := strings.IndexAny(body[k+1:], ">\n")
		if end < 0 || body[k+1+end] != '>' {
			return linkDest{}, 0, false
		}
		d = linkDest{start: k + 1, end: k + 1 + end, angled: true}
		k = k + 1 + end + 1
	} else {
		start, parens := k, 0
		for k < len(body) {
			c := body[k]
			if c == '\\' && k+1 < len(body) {
				k += 2
				continue
			}
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c < 0x20 {
				break
			}
			if c == '(' {
				parens++
			} else if c == ')' {
				if parens == 0 {
					break
				}
				parens--
			}
			k++
		}
		d = linkDest{start: start, end: k}
	}

	// Optional title, then the closing parenthesis.
	k = skipSpace(body, k)
	if k < len(body) && (body[k] == '"' || body[k] == '\'' || body[k] == '(') {
		closer := body[k]
		if closer == '(' {
			closer = ')'
		}
		end := strings.IndexByte(body[k+1:], closer)
		if end < 0 {
			return linkDest{}, 0, false
		}
		k = skipSpace(body, k+1+end+1)
	}
	if k >= len(body) || body[k] != ')' {
		return linkDest{}, 0, false
	}
	d.end = withoutFragment(body, d.start, d.end)
	return d, k + 1, true
}

// refDefinition parses a reference definition "[label]: dest" on one line.
func refDefinition(body string, start, end int) (linkDest, bool) {
	line := body[start:end]
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || !strings.HasPrefix(trimmed, "[") {
		return linkDest{}, false
	}
	off := start + len(line) - len(trimmed)
	close := strings.Index(trimmed, "]:")
	if close < 2 || strings.ContainsAny(trimmed[1:close], "[]") {
		return linkDest{}, false
	}
	k := skipInline(body, off+close+2, end)
	if k >= end {
		return linkDest{}, false
	}
	if body[k] == '<' {
		e := strings.IndexByte(body[k+1:end], '>')
		if e < 0 {
			return linkDest{}, false
		}
		return linkDest{start: k + 1, end: withoutFragment(body, k+1, k+1+e), angled: true}, true
	}
	e := k
	for e < end && body[e] != ' ' && body[e] != '\t' && body[e] != '\r' {
		e++
	}
	return linkDest{start: k, end: withoutFragment(body, k, e)}, true
}

func skipSpace(body string, k int) int {
	for k < len(body) && (body[k] == ' ' || body[k] == '\t' || body[k] == '\n' || body[k] == '\r') {
		k++
	}
	return k
}

func skipInline(body string, k, end int) int {
	for k < end && (body[k] == ' ' || body[k] == '\t') {
		k++
	}
	return k
}

// withoutFragment trims a "#fragment" (and "?query") off a destination range.
func withoutFragment(body string, start, end int) int {
	if i := strings.IndexAny(body[start:end], "#?"); i >= 0 {
		return start + i
	}
	return end
}

// isInternalMarkdown reports whether a destination path names a markdown file
// inside the bundle rather than an external resource.
func isInternalMarkdown(dest string) bool {
	if dest == "" || strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") {
		return false
	}
	return strings.HasSuffix(strings.ToLower(dest), ".md")
}
