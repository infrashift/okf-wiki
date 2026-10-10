package graph

import "strings"

// Footnotes returns the labels body cites as footnote references ([^label])
// and the labels it defines ([^label]: text), each deduplicated in order of
// first appearance. Labels are returned as written; markdown matches them
// case-insensitively, so compare with FootnoteKey.
//
// Fenced code, inline code spans and backslash escapes are skipped the way the
// link scanner skips them: `arr[^0]` in an example is not a citation. A
// definition's own leading [^label] is not counted as a reference to itself.
func Footnotes(body string) (refs, defs []string) {
	// Validation runs this on every body, and most cite nothing.
	if !strings.Contains(body, "[^") {
		return nil, nil
	}
	fences := fencedRanges(body)
	defAt := make(map[int]bool)
	seenDef := make(map[string]bool)
	for _, ln := range lines(body) {
		if inRanges(fences, ln[0]) {
			continue
		}
		// Any indent, not CommonMark's three spaces: a definition nested in a
		// list item sits deeper, and so does a [^x]: example in an indented
		// code block, which this scanner cannot tell from one (see linkDests).
		// Taking either as a definition errs towards a citation resolving,
		// which is the direction a validator must err in.
		line := body[ln[0]:ln[1]]
		t := strings.TrimLeft(line, " \t")
		at := ln[0] + len(line) - len(t)
		if label, end, ok := footnoteLabel(body, at); ok && end < len(body) && body[end] == ':' {
			defAt[at] = true
			if !seenDef[FootnoteKey(label)] {
				seenDef[FootnoteKey(label)] = true
				defs = append(defs, label)
			}
		}
	}

	seenRef := make(map[string]bool)
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
			label, end, ok := footnoteLabel(body, i)
			// [^x](url) is an inline link whose text happens to start with ^.
			if ok && !defAt[i] && (end >= len(body) || body[end] != '(') {
				if !seenRef[FootnoteKey(label)] {
					seenRef[FootnoteKey(label)] = true
					refs = append(refs, label)
				}
				i = end
			} else {
				i++
			}
		default:
			i++
		}
	}
	return refs, defs
}

// footnoteLabel parses [^label] at body[i], returning the label and the offset
// just past the closing bracket. A label is non-empty and holds no whitespace,
// brackets or backslashes.
func footnoteLabel(body string, i int) (string, int, bool) {
	if !strings.HasPrefix(body[i:], "[^") {
		return "", 0, false
	}
	for j := i + 2; j < len(body); j++ {
		switch body[j] {
		case ']':
			if j == i+2 {
				return "", 0, false
			}
			return body[i+2 : j], j + 1, true
		case ' ', '\t', '\n', '\r', '[', '\\':
			return "", 0, false
		}
	}
	return "", 0, false
}

// FootnoteKey is the form two footnote labels are compared in. Markdown
// matches labels case-insensitively, so [^GA4] and [^ga4] are one footnote.
func FootnoteKey(label string) string {
	return strings.ToLower(label)
}
