// Package bundle reads an OKF bundle, the .okfw/ directory that okfw writes,
// from any fs.FS. It is the code okfw itself uses to find, parse and search
// concepts, so a program that reads a bundle through this package sees what
// `okfw search` and `okfw validate` see.
//
// The package only reads. It never writes to the FS, and it holds no state
// between calls.
//
// A typical consumer has a bundle in memory, for example extracted from an
// archive, and loads it once:
//
//	sub, err := fs.Sub(archiveFS, ".okfw")    // the bundle root
//	concepts, err := bundle.Load(sub, bundle.DefaultLimits)
//	hits := bundle.Search(concepts, "token refresh", 25)
//
// Treat bundle content as untrusted input. Walk and Load bound every read (see
// Limits), and Parse and Search never execute anything a concept contains.
package bundle

import (
	"github.com/infrashift/okf-wiki/internal/config"
)

// Concept is one concept of a bundle: the frontmatter fields a reader needs,
// and the markdown body.
type Concept struct {
	// Path is the concept's file, slash-separated and relative to the bundle
	// root: "decisions/adr-0001.md".
	Path string
	// ID is the frontmatter id. A concept without one is identified by its OKF
	// concept ID instead, which is Path without ".md" (OKF section 2). okfw
	// search reports the same value.
	ID string

	Title       string
	Type        string // OKF's free-form concept type
	Description string // a one-sentence summary; may be empty
	Tags        []string

	// Status is the OKF lifecycle status: draft, stable or deprecated. An
	// absent or unrecognised value reads as stable (OKF section 5.4).
	Status string

	// Body is the markdown after the frontmatter block.
	Body string
}

// Parse reads the concept in content, the bytes of the file at path (relative
// to the bundle root, used for Path and for ID when the frontmatter has none).
//
// Parse is lenient, as every okfw read is: a field with the wrong shape, such
// as `tags: foo`, is left empty instead of failing the concept. It returns an
// error only when content has no frontmatter block, or the frontmatter is not
// a YAML mapping. Use Validate for the strict check.
func Parse(path string, content []byte) (*Concept, error) {
	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return nil, err
	}
	return &Concept{
		Path:        path,
		ID:          config.ConceptKey(z, path),
		Title:       z.Title,
		Type:        z.Type,
		Description: z.Description,
		Tags:        z.Tags,
		Status:      z.LifecycleStatus(),
		Body:        config.ExtractBody(content),
	}, nil
}

// Validate applies the strict check that `okfw validate` applies to one
// concept: the frontmatter must satisfy okfw's CUE #Concept schema (which
// requires id, title, type, tags, created and timestamp, and rejects
// undeclared keys outside x:), the business rules the schema cannot express,
// and every [^label] footnote in the body must resolve.
//
// okfw's schema is deliberately stricter than OKF, so a conformant bundle
// that okfw did not write can fail it. `okfw validate --all` also reports two
// files that share an id, which needs the whole bundle and is not checked
// here.
func Validate(content []byte) error {
	_, err := config.ParseAndValidate(content)
	return err
}
