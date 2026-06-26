package graphdb

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	bolt "go.etcd.io/bbolt"
)

//go:embed edge-schema.cue
var edgeSchema string

// LoadEdgeSchema compiles and returns the #Edge CUE definition.
func LoadEdgeSchema() (cue.Value, error) {
	ctx := cuecontext.New()
	val := ctx.CompileString(edgeSchema)
	if val.Err() != nil {
		return cue.Value{}, fmt.Errorf("failed to compile edge schema: %w", val.Err())
	}
	return val.LookupPath(cue.ParsePath("#Edge")), nil
}

// ValidateEdge validates a single edge record against the CUE schema.
func ValidateEdge(e EdgeRecord) error {
	schema, err := LoadEdgeSchema()
	if err != nil {
		return err
	}

	ctx := schema.Context()

	data := map[string]interface{}{
		"from":      e.From,
		"predicate": e.Predicate,
		"to":        e.To,
	}

	dataVal := ctx.Encode(data)
	if dataVal.Err() != nil {
		return fmt.Errorf("failed to encode edge: %w", dataVal.Err())
	}

	unified := schema.Unify(dataVal)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("edge validation failed: %w", err)
	}

	return nil
}

// ValidateEdges validates a slice of edge records against the CUE schema.
func ValidateEdges(edges []EdgeRecord) []error {
	var errs []error
	for _, e := range edges {
		if err := ValidateEdge(e); err != nil {
			errs = append(errs, fmt.Errorf("edge %s -[%s]-> %s: %w", e.From, e.Predicate, e.To, err))
		}
	}
	return errs
}

// IntegrityResult reports referential integrity issues.
type IntegrityResult struct {
	DanglingEdges []EdgeRecord
	InvalidEdges  []string // CUE validation errors
	TotalEdges    int
	TotalNodes    int
}

// ValidateIntegrity checks all edges for structural validity (CUE) and
// referential integrity (targets exist in the nodes bucket).
func (s *Store) ValidateIntegrity() (*IntegrityResult, error) {
	result := &IntegrityResult{}

	err := s.db.View(func(tx *bolt.Tx) error {
		edges := tx.Bucket(bucketEdges)
		nodes := tx.Bucket(bucketNodes)

		result.TotalNodes = nodes.Stats().KeyN

		return edges.ForEach(func(_, v []byte) error {
			result.TotalEdges++

			var e EdgeRecord
			if err := json.Unmarshal(v, &e); err != nil {
				result.InvalidEdges = append(result.InvalidEdges,
					fmt.Sprintf("malformed edge data: %v", err))
				return nil
			}

			// Structural validation.
			if err := ValidateEdge(e); err != nil {
				result.InvalidEdges = append(result.InvalidEdges, err.Error())
			}

			// Referential integrity (skip tags — they point to tag strings, not node IDs).
			if e.Predicate != PredicateTagged {
				if nodes.Get([]byte(e.To)) == nil {
					result.DanglingEdges = append(result.DanglingEdges, e)
				}
			}

			return nil
		})
	})

	return result, err
}

// String returns a human-readable summary of the integrity result.
func (r *IntegrityResult) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Nodes: %d, Edges: %d\n", r.TotalNodes, r.TotalEdges))

	if len(r.DanglingEdges) == 0 && len(r.InvalidEdges) == 0 {
		sb.WriteString("No integrity issues found.\n")
		return sb.String()
	}

	if len(r.DanglingEdges) > 0 {
		sb.WriteString(fmt.Sprintf("\nDangling edges (%d):\n", len(r.DanglingEdges)))
		for _, e := range r.DanglingEdges {
			sb.WriteString(fmt.Sprintf("  %s -[%s]-> %s (target not found)\n", e.From, e.Predicate, e.To))
		}
	}

	if len(r.InvalidEdges) > 0 {
		sb.WriteString(fmt.Sprintf("\nInvalid edges (%d):\n", len(r.InvalidEdges)))
		for _, msg := range r.InvalidEdges {
			sb.WriteString(fmt.Sprintf("  %s\n", msg))
		}
	}

	return sb.String()
}
