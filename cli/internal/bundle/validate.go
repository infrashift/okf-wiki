package bundle

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"okfw/cli/internal/config"
)

// Validation on the write path has two different jobs, so it has two entry
// points.
//
// ValidateGenerated is for content okfw just produced: there is no excuse for
// okfw writing a concept its own schema rejects, so this is unconditional.
//
// guardMutation is for edits to a file the user already had. Rejecting every
// mutation of an invalid concept would trap the user — they could not add a tag
// or fix a field through the CLI on a file that is already broken. So a mutation
// is refused only when it would turn a *valid* concept invalid. Already-broken
// files stay editable, and `okfw validate` remains the way to find them.

// LegacyTodoFields are the frontmatter keys of okfw's removed task-tracking
// feature. `status` is deliberately absent: OKF v0.2 spec'd it as the lifecycle
// field, so a legacy todo value there is rewritten rather than dropped.
var LegacyTodoFields = []string{"due", "completed", "priority"}

// legacyTodoStatuses are the values okfw's old todo feature wrote into `status`,
// none of which is a valid OKF lifecycle value.
var legacyTodoStatuses = map[string]bool{"open": true, "in_progress": true, "closed": true}

// StripLegacyTodoFields removes the removed todo feature's frontmatter from a
// concept, returning the rewritten content and the keys it dropped. When
// nothing needs changing it returns (nil, nil) so callers can skip the write.
//
// A legacy `status` (open/in_progress/closed) is not simply deleted: the key now
// belongs to OKF's lifecycle family, and a closed todo is most faithfully a
// deprecated concept. Everything else maps to the "stable" default by omission.
func StripLegacyTodoFields(content []byte) ([]byte, []string, error) {
	z, err := config.ParseFrontmatter(content)
	if err != nil {
		return nil, nil, err
	}

	var dropped []string
	updates := map[string]string{}
	remove := []string{}

	if legacyTodoStatuses[z.Status] {
		dropped = append(dropped, "status="+z.Status)
		if z.Status == "closed" {
			updates["status"] = "deprecated"
		} else {
			remove = append(remove, "status") // absent means stable
		}
	}
	if z.Kind == "todo" {
		dropped = append(dropped, "kind=todo")
		remove = append(remove, "kind")
	}

	// The removed fields are gone from the Concept struct, so read them off the
	// raw frontmatter instead.
	present, err := presentKeys(content, LegacyTodoFields)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range present {
		dropped = append(dropped, k)
		remove = append(remove, k)
	}

	if len(dropped) == 0 {
		return nil, nil, nil
	}

	// The timestamp records the last meaningful change; dropping retired
	// bookkeeping is not one, so preserve it.
	updates["timestamp"] = z.Timestamp.Format(time.RFC3339)

	out, err := UpdateFrontmatterRemove(content, updates, remove)
	if err != nil {
		return nil, nil, err
	}
	return out, dropped, nil
}

// presentKeys reports which of the given top-level frontmatter keys exist.
func presentKeys(content []byte, keys []string) ([]string, error) {
	fm, _, _, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(fm, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	var found []string
	for _, k := range keys {
		if MapGet(doc.Content[0], k) != nil {
			found = append(found, k)
		}
	}
	return found, nil
}

// ValidateGenerated reports whether okfw-generated concept markdown satisfies
// the concept schema.
func ValidateGenerated(content []byte) error {
	if _, err := config.ParseAndValidate(content); err != nil {
		return fmt.Errorf("generated concept is not schema-valid: %w", err)
	}
	return nil
}

// guardMutation rejects an edit that would break a concept that validated
// before. It returns nil when the result is valid, and also when the original
// was already invalid.
func guardMutation(before, after []byte) error {
	if _, err := config.ParseAndValidate(after); err == nil {
		return nil
	} else if _, preErr := config.ParseAndValidate(before); preErr != nil {
		// Already invalid going in — the mutation is not what broke it.
		return nil
	} else {
		return fmt.Errorf("edit would make the concept schema-invalid: %w", err)
	}
}
