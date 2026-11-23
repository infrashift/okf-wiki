package graph

import "testing"

func TestNormalizeRefKeepsOnlyConceptPaths(t *testing.T) {
	for _, tc := range []struct{ value, fromDir, want string }{
		{"/tables/orders.md", "finance", "tables/orders.md"},
		{"../notes/derived.md#method", "finance", "notes/derived.md"},
		{"computations/revenue.md", "finance", "finance/computations/revenue.md"}, // bare: from the file's directory (v0.3)
		{"/computations/revenue.md", "finance", "computations/revenue.md"},
		{"./computations/revenue.md", "finance", "finance/computations/revenue.md"},
		{" ./a.md ", "", "a.md"},
		{"https://example.com/x.md", "", ""},          // a URL
		{"all queries in project X", "", ""},          // a scope descriptor
		{"references/attesters/check.py", "", ""},     // a file, not a concept
		{"bigquery://project.dataset.orders", "", ""}, // the asset a concept describes
		{"../../outside.md", "a", ""},                 // climbs out of the bundle
		{"", "", ""},
	} {
		if got := NormalizeRef(tc.value, tc.fromDir); got != tc.want {
			t.Errorf("NormalizeRef(%q, %q) = %q, want %q", tc.value, tc.fromDir, got, tc.want)
		}
	}
}

// OKF v0.3 section 6.2: a bare value that does not resolve from its file's
// directory MAY be tried from the bundle root, in a bundle declaring 0.2 or
// earlier (exists non-nil). Never for "/", "./" or "../" values.
func TestResolveRefLegacyFallback(t *testing.T) {
	files := map[string]bool{"skills/run.md": true, "computations/skills/local.md": true}
	exists := func(rel string) bool { return files[rel] }
	for _, tc := range []struct {
		value, fromDir string
		exists         func(string) bool
		want           string
	}{
		{"skills/run.md", "computations", exists, "skills/run.md"},                  // fallback to the root
		{"skills/local.md", "computations", exists, "computations/skills/local.md"}, // resolves locally first
		{"skills/run.md", "computations", nil, "computations/skills/run.md"},        // v0.3 bundle: no fallback
		{"./skills/run.md", "computations", exists, "computations/skills/run.md"},   // explicit relative: no fallback
		{"skills/none.md", "computations", exists, "computations/skills/none.md"},   // dangling either way
	} {
		if got := ResolveRef(tc.value, tc.fromDir, tc.exists); got != tc.want {
			t.Errorf("ResolveRef(%q, %q) = %q, want %q", tc.value, tc.fromDir, got, tc.want)
		}
	}
}
