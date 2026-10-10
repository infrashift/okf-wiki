package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Makefile and CI used to gate schema validity on
// `cue vet cli/internal/config/concept-schema.cue`, which type-checks the schema
// file against no data instance at all. It cannot catch a concept that violates
// #Concept — the only thing worth gating. This walks the fixture corpus instead,
// so the check runs in the normal `go test` suite and actually asserts something.

const fixtureDir = "../../testdata"

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
