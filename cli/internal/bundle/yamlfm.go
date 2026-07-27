package bundle

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// This file holds the frontmatter editing primitives. Everything that mutates a
// concept's frontmatter goes through Edit, which round-trips the YAML as a
// yaml.Node rather than rewriting lines.
//
// The line-oriented editor this replaced matched fields with
// strings.HasPrefix(line, field+":"), which could only ever see top-level keys —
// it could not touch okfw.status, generated.at, verified[], or sources[]. It
// also scanned with a bufio.Scanner whose 64KB default line cap was never
// checked, so a long frontmatter line silently truncated the block and the
// truncated version was written back.
//
// Round-tripping through yaml.Node preserves key order, comments, unrecognized
// keys, and each scalar's original quoting style, so an edit shows up as one
// changed line rather than a reformat of the whole block.

// docIndent matches okfw's house style: two spaces, so nested keys and sequence
// items line up with what the generators emit.
const docIndent = 2

// splitFrontmatter divides markdown into the raw YAML block, the exact bytes of
// the closing delimiter line, and everything after it. Reassembly is
// "---\n" + yaml + closing + rest, which keeps CRLF files byte-faithful.
//
// Unlike config.ExtractFrontmatter, the closing delimiter must be a line that is
// exactly "---": a value whose text happens to contain "\n---" no longer ends
// the block early.
func splitFrontmatter(content []byte) (fm, closing, rest []byte, err error) {
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, nil, nil, fmt.Errorf("no frontmatter found")
	}
	nl := bytes.IndexByte(content, '\n')
	if nl == -1 {
		return nil, nil, nil, fmt.Errorf("frontmatter closing --- not found")
	}
	if strings.TrimRight(string(content[:nl]), "\r") != "---" {
		return nil, nil, nil, fmt.Errorf("no frontmatter found")
	}

	body := content[nl+1:]
	for off := 0; off <= len(body); {
		var line []byte
		lineEnd := bytes.IndexByte(body[off:], '\n')
		if lineEnd == -1 {
			line = body[off:]
		} else {
			line = body[off : off+lineEnd]
		}
		if strings.TrimRight(string(line), "\r") == "---" {
			return body[:off], line, body[off+len(line):], nil
		}
		if lineEnd == -1 {
			break
		}
		off += lineEnd + 1
	}
	return nil, nil, nil, fmt.Errorf("frontmatter closing --- not found")
}

// Edit parses a concept's frontmatter, hands the top-level mapping node to fn,
// and rewrites the file with the result. The body is passed through untouched.
func Edit(content []byte, fn func(m *yaml.Node) error) ([]byte, error) {
	fm, closing, rest, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	// An empty or comment-only block decodes to nothing; start a mapping so
	// callers can add to it.
	var m *yaml.Node
	if doc.Kind == 0 || len(doc.Content) == 0 {
		m = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{m}}
	} else {
		m = doc.Content[0]
		if m.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("frontmatter must be a YAML mapping")
		}
	}

	if err := fn(m); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(docIndent)
	if err := enc.Encode(&doc); err != nil {
		return nil, fmt.Errorf("failed to encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("failed to encode frontmatter: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(buf.Bytes())
	out.Write(closing)
	out.Write(rest)
	return out.Bytes(), nil
}

// QuotedScalar builds a double-quoted string node, okfw's house style for
// generated values.
func QuotedScalar(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle}
}

// mapEntry returns the value node for key and the index of its key node within
// m.Content, or (nil, -1) when absent.
func mapEntry(m *yaml.Node, key string) (*yaml.Node, int) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1], i
		}
	}
	return nil, -1
}

// MapGet returns the value node for a top-level key, or nil.
func MapGet(m *yaml.Node, key string) *yaml.Node {
	v, _ := mapEntry(m, key)
	return v
}

// MapSet sets key to val, replacing an existing value in place (so key order is
// preserved) or appending a new pair.
func MapSet(m *yaml.Node, key string, val *yaml.Node) {
	if _, i := mapEntry(m, key); i >= 0 {
		m.Content[i+1] = val
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		val)
}

// MapSetString sets key to a string value. An existing scalar keeps its quoting
// style; a new key gets okfw's double-quoted house style.
func MapSetString(m *yaml.Node, key, value string) {
	if cur, _ := mapEntry(m, key); cur != nil && cur.Kind == yaml.ScalarNode {
		cur.Value = value
		cur.Tag = "!!str"
		if cur.Style == 0 {
			cur.Style = yaml.DoubleQuotedStyle
		}
		return
	}
	MapSet(m, key, QuotedScalar(value))
}

// MapDelete removes key and its value. Reports whether anything was removed.
func MapDelete(m *yaml.Node, key string) bool {
	if _, i := mapEntry(m, key); i >= 0 {
		m.Content = append(m.Content[:i], m.Content[i+2:]...)
		return true
	}
	return false
}

// mappingAt returns the mapping node at key, creating one when absent. It errors
// when key exists but holds something other than a mapping.
func mappingAt(m *yaml.Node, key string) (*yaml.Node, error) {
	if cur, _ := mapEntry(m, key); cur != nil {
		if cur.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("frontmatter key %q is not a mapping", key)
		}
		return cur, nil
	}
	sub := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	MapSet(m, key, sub)
	return sub, nil
}

// SetPath sets a dotted path such as "okfw.status", creating intermediate
// mappings as needed.
func SetPath(m *yaml.Node, path, value string) error {
	keys := strings.Split(path, ".")
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, err := mappingAt(cur, k)
		if err != nil {
			return err
		}
		cur = next
	}
	MapSetString(cur, keys[len(keys)-1], value)
	return nil
}

// GetPath resolves a dotted path to its value node, or nil when any segment is
// missing.
func GetPath(m *yaml.Node, path string) *yaml.Node {
	cur := m
	for _, k := range strings.Split(path, ".") {
		if cur == nil || cur.Kind != yaml.MappingNode {
			return nil
		}
		cur = MapGet(cur, k)
	}
	return cur
}

// DeletePath removes the value at a dotted path, pruning any intermediate
// mapping the removal leaves empty. Reports whether anything was removed.
func DeletePath(m *yaml.Node, path string) bool {
	keys := strings.Split(path, ".")
	if len(keys) == 1 {
		return MapDelete(m, keys[0])
	}
	parent := GetPath(m, strings.Join(keys[:len(keys)-1], "."))
	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}
	if !MapDelete(parent, keys[len(keys)-1]) {
		return false
	}
	if len(parent.Content) == 0 {
		DeletePath(m, strings.Join(keys[:len(keys)-1], "."))
	}
	return true
}

// AppendToSeq appends val to the sequence at key, creating the sequence when
// absent. An empty flow sequence ("tags: []") becomes a block sequence so the
// added entries render as a list.
func AppendToSeq(m *yaml.Node, key string, val *yaml.Node) error {
	cur, _ := mapEntry(m, key)
	if cur == nil {
		MapSet(m, key, &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{val}})
		return nil
	}
	// A null value ("tags:" with nothing after it) is an empty sequence.
	if cur.Kind == yaml.ScalarNode && cur.Tag == "!!null" {
		*cur = yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{val}}
		return nil
	}
	if cur.Kind != yaml.SequenceNode {
		return fmt.Errorf("frontmatter key %q is not a sequence", key)
	}
	cur.Content = append(cur.Content, val)
	if len(cur.Content) > 0 {
		cur.Style = 0 // drop flow style so the list renders as a block
	}
	return nil
}

// SeqStrings returns the string values of the sequence at key.
func SeqStrings(m *yaml.Node, key string) []string {
	cur, _ := mapEntry(m, key)
	if cur == nil || cur.Kind != yaml.SequenceNode {
		return nil
	}
	out := make([]string, 0, len(cur.Content))
	for _, n := range cur.Content {
		if n.Kind == yaml.ScalarNode {
			out = append(out, n.Value)
		}
	}
	return out
}
