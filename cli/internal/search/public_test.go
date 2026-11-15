package search

import (
	"os"
	"strings"
	"testing"

	okfbundle "github.com/infrashift/okf-wiki/pkg/bundle"
)

// TestSearchMatchesThePublicPackage: a consumer of pkg/bundle loading a bundle
// and searching it must get what `okfw search` returns, hit for hit. The two
// share the walk and the ranking; this pins that they stay shared.
func TestSearchMatchesThePublicPackage(t *testing.T) {
	root := bundleWith(t)
	write(t, root, "noid.md", "---\ntitle: \"Policy without an id\"\ntype: Guide\ntags: [policy]\n---\nNo id here.\n")
	write(t, root, "deep/c--0003.md", concept("019680f0-7e00-7abc-8def-300000000000",
		"Engines", "Policy engines compared", "An engine evaluates input against a POLICY.\n", ""))
	write(t, root, "ephemeral/x.md", concept("019680f0-7e00-7abc-8def-400000000000", "Policy export", "", "policy\n", ""))
	write(t, root, ".hidden/y.md", concept("019680f0-7e00-7abc-8def-500000000000", "Policy hidden", "", "policy\n", ""))

	concepts, err := okfbundle.Load(os.DirFS(root), okfbundle.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"policy", "policy evaluates", "engine", "", "nothing-matches"} {
		cli, total, err := Search(root, Options{Query: q, Limit: 100})
		if err != nil {
			t.Fatal(err)
		}
		pub := okfbundle.Search(concepts, q, 100)
		if q == "policy" && len(pub) != 4 {
			t.Fatalf("query policy: %d hits, want 4 (none from ephemeral/ or .hidden/)", len(pub))
		}
		if total != len(pub) || len(cli) != len(pub) {
			t.Fatalf("query %q: okfw search found %d (total %d), the package %d", q, len(cli), total, len(pub))
		}
		for i := range cli {
			c, p := cli[i], pub[i]
			if c.ID != p.Concept.ID || c.Score != p.Score || c.Snippet != p.Snippet || c.Title != p.Concept.Title {
				t.Errorf("query %q, hit %d: okfw search %s %v %q, the package %s %v %q",
					q, i, c.ID, c.Score, c.Snippet, p.Concept.ID, p.Score, p.Snippet)
			}
			if !strings.HasSuffix(c.FilePath, p.Concept.Path) {
				t.Errorf("query %q, hit %d: file %s is not %s", q, i, c.FilePath, p.Concept.Path)
			}
		}
	}
}
