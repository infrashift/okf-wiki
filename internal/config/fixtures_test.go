package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The Makefile and CI used to gate schema validity on
// `cue vet internal/config/concept-schema.cue`, which type-checks the schema
// file against no data instance at all. It cannot catch a concept that violates
// #Concept — the only thing worth gating. This walks the fixture corpus instead,
// so the check runs in the normal `go test` suite and actually asserts something.

const fixtureDir = "../../cli/testdata"

func TestEveryValidFixtureValidates(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("cannot read %s: %v", fixtureDir, err)
	}

	var checked int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || !strings.HasPrefix(name, "valid_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(fixtureDir, name))
			if err != nil {
				t.Fatalf("fixture missing: %v", err)
			}
			if _, err := ParseAndValidate(content); err != nil {
				t.Errorf("%s should validate: %v", name, err)
			}
		})
		checked++
	}
	// A corpus that quietly emptied out would otherwise pass.
	if checked < 3 {
		t.Errorf("only %d valid_* fixtures found; the corpus looks truncated", checked)
	}
}

func TestEveryInvalidFixtureIsRejected(t *testing.T) {
	entries, err := os.ReadDir(fixtureDir)
	if err != nil {
		t.Fatalf("cannot read %s: %v", fixtureDir, err)
	}

	var checked int
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || !strings.HasPrefix(name, "invalid_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(fixtureDir, name))
			if err != nil {
				t.Fatalf("fixture missing: %v", err)
			}
			if _, err := ParseAndValidate(content); err == nil {
				t.Errorf("%s should be rejected but validated cleanly", name)
			}
		})
		checked++
	}
	if checked < 1 {
		t.Error("no invalid_* fixtures found; the negative case is untested")
	}
}

// The schema must compile at all — the one thing the old `cue vet` gate did
// verify, kept here so it runs without an external cue binary.
func TestConceptSchemaCompiles(t *testing.T) {
	schema, err := LoadConceptSchema()
	if err != nil {
		t.Fatalf("schema does not compile: %v", err)
	}
	if err := schema.Err(); err != nil {
		t.Fatalf("#Concept definition is not usable: %v", err)
	}
}

// TestSchemaEmbedsNoDefinition: from CUE language v0.18 (explicitopen, made
// stable), an embedded definition stays closed to its own fields, so a
// definition embedding one that holds only a pattern rejects every field it
// declares. okfw's v0.7 library still reads the old way, so no test of the
// compiled schema can notice; the cue v0.18 CLI did, failing CI. Embed a
// hidden regular value (_ext) instead: it means the same under both.
func TestSchemaEmbedsNoDefinition(t *testing.T) {
	embedded := regexp.MustCompile(`^\s+(_?#[A-Za-z]\w*)\s*$`)
	for i, line := range strings.Split(conceptSchema, "\n") {
		if m := embedded.FindStringSubmatch(line); m != nil {
			t.Errorf("concept-schema.cue:%d embeds the definition %s; from CUE v0.18 that closes the enclosing definition to %s's fields alone. Embed a hidden regular value instead, as _ext is", i+1, m[1], m[1])
		}
	}
}
