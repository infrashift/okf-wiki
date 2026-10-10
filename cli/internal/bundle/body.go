package bundle

import (
	"bytes"
	"strings"
)

// A concept keeps the line ending it was written with. okfw writes LF, but a
// concept saved on Windows (or by core.autocrlf) is CRLF throughout, and an
// edit that wrote its own lines as LF would leave the file mixed: a diff that
// touches every line it rewrote, and frontmatter some parsers then split
// differently from the body.

// lineEnding reports the line ending content is written in, judged by its
// opening frontmatter delimiter — the one line every concept has.
func lineEnding(content []byte) string {
	if bytes.HasPrefix(content, []byte("---\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// withLineEnding returns b with every line break written as eol, whatever mix
// of LF and CRLF it arrived in.
func withLineEnding(b []byte, eol string) []byte {
	if eol == "\n" && !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	if eol == "\n" {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\n"), []byte(eol))
}

// AppendBody adds text to the end of a concept as a new paragraph, in the
// concept's own line ending.
func AppendBody(content []byte, text string) []byte {
	eol := lineEnding(content)
	trimmed := bytes.TrimRight(content, "\r\n")
	add := withLineEnding([]byte("\n\n"+strings.TrimRight(text, "\r\n")+"\n"), eol)
	out := make([]byte, 0, len(trimmed)+len(add))
	return append(append(out, trimmed...), add...)
}

// SetBody replaces a concept's body with text, after a blank line, in the
// concept's own line ending. The frontmatter block is left byte for byte.
func SetBody(content []byte, text string) ([]byte, error) {
	body := withLineEnding([]byte("\n"+strings.TrimRight(text, "\r\n")+"\n"), lineEnding(content))
	return ReplaceBody(content, string(body))
}

// ReplaceBody swaps a concept's markdown body, leaving every byte of the
// frontmatter block — including its line endings — exactly as it was.
//
// The body offset is computed rather than rebuilt from the parsed pieces:
// reassembling "---\n" + yaml + "---\n" would normalise a CRLF file's delimiters
// to LF, rewriting lines the caller never asked to touch.
func ReplaceBody(content []byte, newBody string) ([]byte, error) {
	_, _, rest, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}
	// rest starts at the closing delimiter's line ending; the body follows it,
	// matching where config.ExtractBody considers the body to begin.
	bodyStart := len(content) - len(rest)
	switch {
	case strings.HasPrefix(string(rest), "\r\n"):
		bodyStart += 2
	case strings.HasPrefix(string(rest), "\n"):
		bodyStart++
	}
	out := make([]byte, 0, bodyStart+len(newBody))
	out = append(out, content[:bodyStart]...)
	out = append(out, newBody...)
	return out, nil
}
