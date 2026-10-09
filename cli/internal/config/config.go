package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"gopkg.in/yaml.v3"
)

//go:embed concept-schema.cue
var conceptSchema string

// WikiDirName is the per-project wiki directory created by `okfw init` and
// discovered by every command.
const WikiDirName = ".okfw"

// Fixed locations inside the wiki directory. Storage is standardized — there
// is no user configuration.
const ()

// Config carries the resolved paths of a discovered wiki.
type Config struct {
	RootPath string // absolute path of the .okfw directory (the OKF bundle root)
}

// ErrNoWiki is returned by Discover when no .okfw directory is found.
type ErrNoWiki struct {
	StartDir string
}

func (e *ErrNoWiki) Error() string {
	return fmt.Sprintf("no OKF wiki found (searched upward from %s); run 'okfw init' at your project root to create one", e.StartDir)
}

// ConfigFor returns the Config for a wiki rooted at the given .okfw directory.
func ConfigFor(wikiDir string) *Config {
	return &Config{
		RootPath: wikiDir,
	}
}

// Discover walks from startDir (or the current working directory when "")
// upward to the nearest directory containing .okfw/ and returns the resolved
// Config. Returns *ErrNoWiki when no wiki is found.
func Discover(startDir string) (*Config, error) {
	if startDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get working directory: %w", err)
		}
		startDir = cwd
	}
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %q: %w", startDir, err)
	}

	dir := abs
	for {
		wikiDir := filepath.Join(dir, WikiDirName)
		if fi, err := os.Stat(wikiDir); err == nil && fi.IsDir() {
			return ConfigFor(wikiDir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, &ErrNoWiki{StartDir: abs}
		}
		dir = parent
	}
}

// The compiled schema is process-wide and immutable, so it is built once.
// Recompiling per call made every concept pay CUE compilation — invisible for a
// single `okfw validate`, but paid thousands of times over by validate --all.
var (
	schemaOnce sync.Once
	schemaVal  cue.Value
	schemaErr  error
)

// LoadConceptSchema returns the #Concept CUE definition, compiling it on first
// use. The returned value is shared; callers must not mutate it.
func LoadConceptSchema() (cue.Value, error) {
	schemaOnce.Do(func() {
		ctx := cuecontext.New()
		val := ctx.CompileString(conceptSchema)
		if val.Err() != nil {
			schemaErr = fmt.Errorf("failed to compile concept schema: %w", val.Err())
			return
		}
		schemaVal = val.LookupPath(cue.ParsePath("#Concept"))
	})
	return schemaVal, schemaErr
}

// ValidateConceptYAML validates YAML frontmatter against the #Concept schema.
func ValidateConceptYAML(frontmatter []byte) error {
	schema, err := LoadConceptSchema()
	if err != nil {
		return err
	}

	// Parse YAML to map
	var data map[string]interface{}
	if err := yaml.Unmarshal(frontmatter, &data); err != nil {
		return fmt.Errorf("failed to parse frontmatter YAML: %w", err)
	}

	// Convert to CUE and validate
	ctx := schema.Context()
	dataVal := ctx.Encode(data)
	if dataVal.Err() != nil {
		return fmt.Errorf("failed to encode frontmatter: %w", dataVal.Err())
	}

	unified := schema.Unify(dataVal)
	// Use Concrete(true) to ensure all required fields have concrete values
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("frontmatter validation failed: %w", err)
	}

	return nil
}
