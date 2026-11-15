package graph

import "strings"

// Heading is one ATX heading in a markdown body.
type Heading struct {
	Line  int    // 0-based index into the body's "\n"-separated lines
	Level int    // 1–6
	Text  string // trimmed, without its closing #s
}

// Headings returns the ATX headings of body, in order. A "#" line inside a
// fenced code block is not one: a shell comment in a ```bash block read as a
// level-1 heading, which ended every enclosing section at the first comment.
func Headings(body string) []Heading {
	fences := fencedRanges(body)
	var out []Heading
	for i, ln := range lines(body) {
		if inRanges(fences, ln[0]) {
			continue
		}
		if h, ok := atxHeading(strings.TrimRight(body[ln[0]:ln[1]], "\r")); ok {
			h.Line = i
			out = append(out, h)
		}
	}
	return out
}

// atxHeading parses line as a CommonMark ATX heading: up to three spaces of
// indent, one to six #s, then a space, a tab, or the end of the line. A
// closing run of #s is dropped when a space separates it from the text.
func atxHeading(line string) (Heading, bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return Heading{}, false
	}
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || (n < len(t) && t[n] != ' ' && t[n] != '\t') {
		return Heading{}, false
	}
	text := strings.TrimSpace(t[n:])
	if closing := strings.TrimRight(text, "#"); closing == "" {
		text = ""
	} else if strings.HasSuffix(closing, " ") || strings.HasSuffix(closing, "\t") {
		text = strings.TrimSpace(closing)
	}
	return Heading{Level: n, Text: text}, true
}
