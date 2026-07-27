package extract

import (
	"strings"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

// Test linking. A test file is not just another source file: it specifies the
// behaviour of something else, and "what tests this?" / "what does this test?"
// are questions an agent asks constantly. Neither was answerable before —
// nothing in extraction, resolution or detection recognised any test
// convention, so a test's calls were indistinguishable from production calls
// and test entities inflated Degree and the community partition alongside the
// code they exercise.
//
// This runs as a Session post-pass for the same reason rationale does: the
// Result already holds the file's definition nodes, which is exactly what
// marking and subject-derivation need, and one pass covers every language
// instead of editing each extractor.

// maxTestRefs caps derived subjects per file, mirroring the other post-pass
// caps. A generated table-driven suite can declare a great many test functions.
const maxTestRefs = 200

// attachTests marks a test file's nodes and derives `tests` refs from test
// function names.
func attachTests(file detect.File, res *Result) {
	if res == nil || len(res.Nodes) == 0 || !detect.IsTestPath(file.Path) {
		return
	}
	for i := range res.Nodes {
		res.Nodes[i].Test = true
	}
	emitted := 0
	for _, n := range res.Nodes {
		if emitted >= maxTestRefs {
			return
		}
		switch n.Kind {
		case model.KindFunction, model.KindMethod:
		default:
			continue
		}
		subject := testSubject(file.Language, n.Label)
		if subject == "" || subject == n.Label {
			continue
		}
		res.Refs = append(res.Refs, model.Ref{
			FromID:   n.ID,
			Name:     subject,
			Relation: model.RelTests,
			Line:     n.Span.StartLine,
		})
		emitted++
	}
}

// testSubject derives the entity a test function name claims to exercise, or ""
// when the name follows no recognised convention.
//
// Only the naming convention is used, deliberately. Promoting a test's call
// edges instead would mark every helper the test happens to touch — assertion
// libraries, fixture builders, the standard library — as being under test,
// which is both wrong and far noisier than saying nothing.
func testSubject(lang, label string) string {
	// A method's own qualifier is not part of the subject: a Go test is a
	// top-level func, and a python test method's class is the suite, not the
	// thing tested.
	if i := strings.LastIndex(label, "."); i >= 0 {
		label = label[i+1:]
	}
	switch lang {
	case "go":
		// Test/Benchmark/Fuzz/Example are the four names `go test` recognises.
		for _, prefix := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
			rest, ok := strings.CutPrefix(label, prefix)
			if !ok || rest == "" {
				continue
			}
			// The convention for a method is TestType_Method; the underscore
			// stands in for the dot the symbol index uses. A trailing
			// _lowercase suffix is a subtest label ("TestFoo_returnsNil"), not
			// a method, so it is dropped rather than turned into a qualifier.
			name, suffix, split := strings.Cut(rest, "_")
			if !split {
				return rest
			}
			if suffix != "" && suffix[0] >= 'A' && suffix[0] <= 'Z' {
				return name + "." + suffix
			}
			return name
		}
	case "python":
		if rest, ok := strings.CutPrefix(label, "test_"); ok {
			return rest
		}
	}
	return ""
}
