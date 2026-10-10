package bundle

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// bareLF reports whether b has a line feed without its carriage return.
func bareLF(b []byte) bool {
	return bytes.Count(b, []byte("\n")) != bytes.Count(b, []byte("\r\n"))
}

const crlfConcept = "---\r\nid: \"a\"\r\ntitle: \"T\" # kept\r\ntags:\r\n  - x\r\n---\r\n\r\n# T\r\n\r\nBody line.\r\n"

// One frontmatter edit used to write the block as LF between a CRLF closing
// delimiter and a CRLF body.
func TestEditKeepsACRLFConceptCRLF(t *testing.T) {
	out, err := Edit([]byte(crlfConcept), func(m *yaml.Node) error {
		MapSetString(m, "status", "draft")
		return AppendToSeq(m, "tags", QuotedScalar("y"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if bareLF(out) {
		t.Errorf("edit mixed line endings into a CRLF concept:\n%q", out)
	}
	for _, want := range []string{"status: \"draft\"\r\n", "# kept\r\n", "  - \"y\"\r\n", "---\r\n\r\n# T\r\n\r\nBody line.\r\n"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in\n%q", want, out)
		}
	}
}

func TestEditLeavesAnLFConceptLF(t *testing.T) {
	in := strings.ReplaceAll(crlfConcept, "\r\n", "\n")
	out, err := Edit([]byte(in), func(m *yaml.Node) error { MapSetString(m, "status", "draft"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("\r")) {
		t.Errorf("edit introduced a CR into an LF concept:\n%q", out)
	}
}

func TestAppendBodyAndSetBodyUseTheConceptsLineEnding(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		in := []byte(strings.ReplaceAll(crlfConcept, "\r\n", eol))
		// The caller's text arrives in whatever ending its client used.
		text := "Added.\r\nSecond line.\n"

		appended := AppendBody(in, text)
		wantTail := strings.ReplaceAll("Body line.\n\nAdded.\nSecond line.\n", "\n", eol)
		if !strings.HasSuffix(string(appended), wantTail) {
			t.Errorf("eol %q: append = %q, want suffix %q", eol, appended, wantTail)
		}

		set, err := SetBody(in, text)
		if err != nil {
			t.Fatal(err)
		}
		fmEnd := strings.Index(string(in), "---"+eol+eol+"# T")
		wantSet := string(in[:fmEnd]) + strings.ReplaceAll("---\n\nAdded.\nSecond line.\n", "\n", eol)
		if string(set) != wantSet {
			t.Errorf("eol %q: set_body = %q, want %q", eol, set, wantSet)
		}

		for name, out := range map[string][]byte{"append": appended, "set_body": set} {
			if eol == "\r\n" && bareLF(out) {
				t.Errorf("%s mixed line endings into a CRLF concept:\n%q", name, out)
			}
			if eol == "\n" && bytes.Contains(out, []byte("\r")) {
				t.Errorf("%s introduced a CR into an LF concept:\n%q", name, out)
			}
		}
	}
}
