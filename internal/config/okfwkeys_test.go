package config

import (
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const legacyConcept = `---
id: "019680f0-7e00-7abc-8def-1234567890ab"
title: "T"
type: "note"
kind: "issue"
project: "p"
parent: "019680f0-7e00-7abc-8def-000000000001"
tags: []
created: "2026-07-01T10:00:00Z"
timestamp: "2026-07-02T10:00:00Z"
---

# T
`

// Reads accept the bare names okfw wrote before the x-okfw- namespace.
func TestLegacyKeysAreRead(t *testing.T) {
	z, err := ParseFrontmatter([]byte(legacyConcept))
	if err != nil {
		t.Fatal(err)
	}
	if z.ID != "019680f0-7e00-7abc-8def-1234567890ab" || z.Kind != "issue" || z.Project != "p" ||
		z.Parent != "019680f0-7e00-7abc-8def-000000000001" ||
		!z.Created.Equal(time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)) ||
		!z.Timestamp.Equal(time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("legacy keys not adopted: %+v", z)
	}
}

// Strict validation refuses the bare names and says how to fix them.
func TestStrictValidateNamesTheMigration(t *testing.T) {
	if _, err := ParseAndValidate([]byte(legacyConcept)); err == nil || !strings.Contains(err.Error(), "okfw migrate") {
		t.Errorf("err = %v, want a pointer to okfw migrate", err)
	}
}

// The new key wins where a file holds both spellings.
func TestNewKeyWinsOverLegacy(t *testing.T) {
	z, err := ParseFrontmatter([]byte("---\ntype: T\nid: \"019680f0-7e00-7abc-8def-1234567890ab\"\nx-okfw-id: \"019680f0-7e00-7abc-8def-0000000000ff\"\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if z.ID != "019680f0-7e00-7abc-8def-0000000000ff" {
		t.Errorf("ID = %q, want the x-okfw-id value", z.ID)
	}
}

func upgrade(t *testing.T, fm string) (string, []string) {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fm), &doc); err != nil {
		t.Fatal(err)
	}
	old := UpgradeLegacyKeys(doc.Content[0])
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), old
}

// Only a concept okfw wrote is relabelled: another producer's bare id or
// kind is its own.
func TestUpgradeTouchesOnlyOkfwConcepts(t *testing.T) {
	for _, foreign := range []string{
		"type: T\nid: orders-table\nkind: table\n",
		"type: T\ncreated: 2026-01-01\n",
	} {
		if out, old := upgrade(t, foreign); old != nil || strings.Contains(out, "x-okfw-") {
			t.Errorf("a foreign concept was relabelled: %v\n%s", old, out)
		}
	}
	out, old := upgrade(t, "id: \"019680f0-7e00-7abc-8def-1234567890ab\" # mine\ntitle: T\ntimestamp: \"2026-07-02T10:00:00Z\"\nx-okfw-created: \"2026-07-01T10:00:00Z\"\ncreated: \"2020-01-01T00:00:00Z\"\n")
	if strings.Join(old, ",") != "id,created,timestamp" {
		t.Errorf("renamed %v", old)
	}
	want := "x-okfw-id: \"019680f0-7e00-7abc-8def-1234567890ab\" # mine\ntitle: T\nx-okfw-timestamp: \"2026-07-02T10:00:00Z\"\nx-okfw-created: \"2026-07-01T10:00:00Z\"\n"
	if out != want {
		t.Errorf("upgraded:\n%s\nwant (in place, comment kept, the existing new key winning):\n%s", out, want)
	}
}
