package bundle_test

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/infrashift/okf-wiki/pkg/bundle"
)

func TestParseReadsTheFields(t *testing.T) {
	content := "---\r\n" +
		"x-okfw-id: \"019680f0-7e00-7abc-8def-1234567890ab\"\r\n" +
		"title: \"Token refresh\"\r\n" +
		"type: ADR\r\n" +
		"description: How tokens are refreshed.\r\n" +
		"tags: [auth, oidc]\r\n" +
		"status: deprecated\r\n" +
		"---\r\n" +
		"# Token refresh\r\n"
	c, err := bundle.Parse("decisions/adr-1.md", []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	want := bundle.Concept{
		Path:        "decisions/adr-1.md",
		ID:          "019680f0-7e00-7abc-8def-1234567890ab",
		Title:       "Token refresh",
		Type:        "ADR",
		Description: "How tokens are refreshed.",
		Tags:        []string{"auth", "oidc"},
		Status:      "deprecated",
		Body:        "# Token refresh\r\n",
	}
	if !reflect.DeepEqual(*c, want) {
		t.Errorf("Parse = %+v\nwant    %+v", *c, want)
	}
}

// TestParseIsLenient: okfw's reads never fail a concept over a field with the
// wrong shape, and a concept without an id is identified by its path. OKF
// v0.3 section 11.2: a scalar tags is one tag, never split at commas, and an
// unknown status is kept raw, since it is not stable (section 5.4).
func TestParseIsLenient(t *testing.T) {
	c, err := bundle.Parse("notes/x.md", []byte("---\ntype: Note\ntitle: X\ntags: foo, bar\nstatus: open\nx-okfw-created: soon\n---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "notes/x" {
		t.Errorf("ID = %q, want the OKF concept ID notes/x", c.ID)
	}
	if len(c.Tags) != 1 || c.Tags[0] != "foo, bar" || c.Title != "X" {
		t.Errorf("a scalar tags is one tag: %+v", c)
	}
	if c.Status != "open" {
		t.Errorf("Status = %q; an unrecognised status is kept as written", c.Status)
	}
}

// TestParseRefusesAnUntypedFile: OKF v0.3 section 3.2 — a .md file with no
// type is not a concept, so a README.md in a bundle is not one.
func TestParseRefusesAnUntypedFile(t *testing.T) {
	if _, err := bundle.Parse("README.md", []byte("---\ntitle: Read me\n---\nbody\n")); !errors.Is(err, bundle.ErrNotConcept) {
		t.Errorf("err = %v, want ErrNotConcept", err)
	}
}

func TestParseRefusesContentWithoutFrontmatter(t *testing.T) {
	for _, content := range []string{"# no frontmatter\n", "---\ntitle: never closed\n", "---\n- a list\n---\n"} {
		if _, err := bundle.Parse("x.md", []byte(content)); err == nil {
			t.Errorf("Parse(%q) succeeded", content)
		}
	}
}

func TestValidateAppliesTheStrictSchema(t *testing.T) {
	valid, err := os.ReadFile("../../cli/testdata/valid_concept.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Validate(valid); err != nil {
		t.Errorf("valid fixture: %v", err)
	}
	for name, content := range map[string]string{
		"missing id and timestamps": "---\ntitle: X\ntype: note\ntags: []\n---\n",
		"undeclared key":            strings.Replace(string(valid), "tags:", "colour: red\ntags:", 1),
		"bad actor":                 strings.Replace(string(valid), "tags:", "generated:\n  by: Human:alice\ntags:", 1),
		"dangling footnote":         string(valid) + "\nA claim.[^nowhere]\n",
	} {
		err := bundle.Validate([]byte(content))
		if err == nil {
			t.Errorf("%s: Validate passed", name)
		}
		t.Logf("%s: %v", name, err)
	}
	// Parse still reads what Validate rejects.
	if _, err := bundle.Parse("x.md", []byte("---\ntitle: X\ntype: note\n---\n")); err != nil {
		t.Errorf("Parse refused a concept okfw would reject: %v", err)
	}
}
