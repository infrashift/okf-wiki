package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/infrashift/okf-wiki/cli/internal/bundle"
	"github.com/infrashift/okf-wiki/cli/internal/search"
)

// copyTree copies the vendored bundle at src to a fresh directory, so a test
// can never write into testdata.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestUpstreamBundlesAreConformantAndFullyLinked runs okfw over the OKF v0.3
// reference repository's example bundles (cli/testdata/upstream, see its
// NOTICE.md). They are the spec authors' own reading of v0.3, so a
// disagreement here is a bug in okfw until shown otherwise: validate --okf
// must find no violation and no broken link, and the graph must leave no
// path field or body link dangling.
func TestUpstreamBundlesAreConformantAndFullyLinked(t *testing.T) {
	for _, name := range []string{"acme_retail", "crypto_bitcoin", "ga4", "stackoverflow"} {
		t.Run(name, func(t *testing.T) {
			root := copyTree(t, filepath.Join("..", "..", "testdata", "upstream", name))
			cfg := testCfg(root)
			out, err := captureStdout(t, func() error { return runValidateOKF(cfg, root) })
			if err != nil {
				t.Fatalf("validate --okf: %v\n%s", err, out)
			}
			if !strings.Contains(out, "0 violation(s), 0 broken link(s)") {
				t.Errorf("validate --okf:\n%s", out)
			}
			g, err := bundle.LoadGraph(root)
			if err != nil {
				t.Fatal(err)
			}
			if d := g.Dangling(); len(d) > 0 {
				t.Errorf("dangling edges: %+v", d)
			}
			hits, _, err := search.Search(root, search.Options{Limit: 1000})
			if err != nil {
				t.Fatal(err)
			}
			if len(hits) == 0 {
				t.Error("search found no concepts")
			}
		})
	}
}

// TestAcmeRetailSupersededByAndTrust checks the v0.3 features acme_retail
// exercises: superseded_by, followed by a current-knowledge search, and the
// current-event trust tier.
func TestAcmeRetailSupersededByAndTrust(t *testing.T) {
	root := copyTree(t, filepath.Join("..", "..", "testdata", "upstream", "acme_retail"))
	hits, _, err := search.Search(root, search.Options{Query: "gross margin legacy", Current: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.Lifecycle() == "deprecated" {
			t.Errorf("--current returned a deprecated concept: %s", h.ID)
		}
	}
	all, _, _ := search.Search(root, search.Options{Limit: 1000})
	var replaced bool
	for _, h := range all {
		if h.SupersededBy != "" {
			replaced = true
		}
	}
	if !replaced {
		t.Error("acme_retail's superseded_by was not read")
	}
}
