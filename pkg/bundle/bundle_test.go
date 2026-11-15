package bundle_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/infrashift/okf-wiki/pkg/bundle"
)

func TestParseReadsTheFields(t *testing.T) {
	content := "---\r\n" +
		"id: \"019680f0-7e00-7abc-8def-1234567890ab\"\r\n" +
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
// wrong shape, and a concept without an id is identified by its path.
func TestParseIsLenient(t *testing.T) {
	c, err := bundle.Parse("notes/x.md", []byte("---\ntitle: X\ntags: foo\nstatus: open\n---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != "notes/x" {
		t.Errorf("ID = %q, want the OKF concept ID notes/x", c.ID)
	}
	if c.Tags != nil || c.Title != "X" {
		t.Errorf("a malformed tags field should cost only itself: %+v", c)
	}
	if c.Status != "stable" {
		t.Errorf("Status = %q; an unrecognised status reads as stable", c.Status)
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
