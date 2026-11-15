package graph

import "testing"

func TestNormalizeRefKeepsOnlyConceptPaths(t *testing.T) {
	for _, tc := range []struct{ value, fromDir, want string }{
		{"/tables/orders.md", "finance", "tables/orders.md"},
		{"../notes/derived.md#method", "finance", "notes/derived.md"},
		{"computations/revenue.md", "finance", "computations/revenue.md"}, // bare: from the bundle root
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
