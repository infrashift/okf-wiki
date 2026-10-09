package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"okfw/cli/internal/config"
)

func TestCheckIndexFile(t *testing.T) {
	for name, tc := range map[string]struct {
		content          string
		root             bool
		violations, warn int
	}{
		"plain listing":            {"# Tables\n\n* [Orders](orders.md) - One row per order.\n* [Sub](sub/)\n", false, 0, 0},
		"root with okf_version":    {"---\nokf_version: \"0.2\"\n---\n\n# /\n\n* [A](a.md)\n", true, 0, 0},
		"frontmatter below root":   {"---\nokf_version: \"0.2\"\n---\n\n# Sub\n", false, 1, 0},
		"root with another key":    {"---\nokf_version: \"0.2\"\ntitle: Home\n---\n\n# /\n", true, 1, 0},
		"concept-shaped index":     {"---\ntype: note\n---\n\n# x\n", false, 1, 0},
		"non-link entry":           {"# Things\n\n* just text\n- [ok](ok.md)\n", false, 0, 1},
		"bullet in a code example": {"# Things\n\n```\n* not a listing\n```\n", false, 0, 0},
		"no heading":               {"* [A](a.md)\n", false, 0, 1},
	} {
		v, w := CheckIndexFile([]byte(tc.content), tc.root)
		if len(v) != tc.violations || len(w) != tc.warn {
			t.Errorf("%s: violations %q, warnings %q; want %d and %d", name, v, w, tc.violations, tc.warn)
		}
	}
}

// okfw's own listings must pass the check it applies to everyone else's,
// including the grouped form, which used to end each group with a non-link
// bullet.
func TestGeneratedListingsAreConformant(t *testing.T) {
	root := t.TempDir()
	cfg := config.ConfigFor(root)
	for i := range 60 {
		id := fmt.Sprintf("019680f0-7e00-7abc-8def-%012d", i)
		writeConcept(t, root, fmt.Sprintf("big/c%02d--0001.md", i), conceptWithType(id, fmt.Sprintf("C %02d", i), []string{"A", "B"}[i%2]))
	}
	writeConcept(t, root, "small/one--0001.md", conceptWithType("019680f0-7e00-7abc-8def-100000000000", "One", "note"))
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"", "big", "small"} {
		content, err := os.ReadFile(filepath.Join(root, dir, "index.md"))
		if err != nil {
			t.Fatal(err)
		}
		if v, w := CheckIndexFile(content, dir == ""); len(v)+len(w) > 0 {
			t.Errorf("%s/index.md: violations %q, warnings %q\n%s", dir, v, w, content)
		}
	}
}

func TestCheckLogFile(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		want    int
	}{
		"newest first":   {"# Log\n\n## 2026-05-22\n* **Update**: x\n\n## 2026-05-15\n* **Creation**: y\n", 0},
		"same day twice": {"## 2026-05-22\n* a\n## 2026-05-22\n* b\n", 0},
		"not a date":     {"## yesterday\n* a\n", 1},
		"date-time":      {"## 2026-05-22T10:00\n* a\n", 1},
		"impossible":     {"## 2026-13-40\n* a\n", 1},
		"out of order":   {"## 2026-02-01\n* a\n## 2026-03-01\n* b\n", 1},
		"frontmatter":    {"---\ntype: log\n---\n## 2026-02-01\n", 1},
	} {
		if got := CheckLogFile([]byte(tc.content)); len(got) != tc.want {
			t.Errorf("%s: %q, want %d violation(s)", name, got, tc.want)
		}
	}
}

func TestDeclaredOKFVersion(t *testing.T) {
	if v := DeclaredOKFVersion([]byte("---\nokf_version: \"1.0\"\n---\n# /\n")); v != "1.0" {
		t.Errorf("got %q", v)
	}
	if v := DeclaredOKFVersion([]byte("# /\n")); v != "" {
		t.Errorf("got %q", v)
	}
}
