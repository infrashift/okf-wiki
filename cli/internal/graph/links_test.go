package graph

import (
	"reflect"
	"strings"
	"testing"
)

// Each case is a construct the regex scanner got wrong: either a real link it
// missed (so moves broke it and backlinks never saw it) or text it took for a
// link (so moves rewrote example prose).
func TestExtractLinkTargetsFollowsTheMarkdownGrammar(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want []string
	}{
		"plain":                {"see [a](a.md).", []string{"a.md"}},
		"title":                {`[a](a.md "The A")`, []string{"a.md"}},
		"single-quote title":   {`[a](a.md 'The A')`, []string{"a.md"}},
		"paren title":          {`[a](a.md (The A))`, []string{"a.md"}},
		"angle brackets":       {"[a](<my notes/a b.md>)", []string{"my notes/a b.md"}},
		"percent-encoded":      {"[a](my%20notes/a%2Db.md)", []string{"my%20notes/a%2Db.md"}},
		"fragment":             {"[a](/dir/a.md#section)", []string{"/dir/a.md"}},
		"upper-case suffix":    {"[a](A.MD)", []string{"A.MD"}},
		"reference def":        {"Body [a][r].\n\n[r]: ./a.md \"title\"\n", []string{"./a.md"}},
		"indented ref def":     {"   [r]: <./a b.md>\n", []string{"./a b.md"}},
		"image":                {"![diagram](a.md)", []string{"a.md"}},
		"nested brackets":      {"[see [the] doc](a.md)", []string{"a.md"}},
		"wrapped link text":    {"a [long\ntitle](a.md) b", []string{"a.md"}},
		"inside outer text":    {"[x [inner](a.md) y]", []string{"a.md"}},
		"balanced parens":      {"[a](dir/a(1).md)", []string{"dir/a(1).md"}},
		"fenced backticks":     {"```\n[a](a.md)\n```\n[b](b.md)", []string{"b.md"}},
		"fenced tildes":        {"~~~md\n[a](a.md)\n~~~\n", nil},
		"unclosed fence":       {"```\n[a](a.md)\n", nil},
		"longer closing":       {"````\n```\n[a](a.md)\n````\n[b](b.md)", []string{"b.md"}},
		"inline code":          {"use `[a](a.md)` to link", nil},
		"double-tick code":     {"use ``[a](a.md) ` x`` here [b](b.md)", []string{"b.md"}},
		"escaped bracket":      {`\[a](a.md)`, nil},
		"blank line in text":   {"[a\n\nb](a.md)", nil},
		"external":             {"[a](https://x.com/a.md) [b](mailto:b.md)", nil},
		"not markdown":         {"[a](a.png) [b](b.md.bak)", nil},
		"reference use only":   {"[a][r]", nil},
		"dedup in order":       {"[b](b.md) [a](a.md) [b2](b.md)", []string{"b.md", "a.md"}},
		"ref def in a fence":   {"```\n[r]: a.md\n```\n", nil},
		"ordered across kinds": {"[r]: z.md\n\n[a](a.md)", []string{"z.md", "a.md"}},
	} {
		if got := ExtractLinkTargets(tc.body); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestRewriteLinksLeavesCodeAlone(t *testing.T) {
	body := "Link [a](a.md).\n\n```\n[a](a.md)\n```\n\nInline `[a](a.md)`.\n"
	got := RewriteLinks(body, func(string) string { return "b/a.md" })
	want := "Link [a](b/a.md).\n\n```\n[a](a.md)\n```\n\nInline `[a](a.md)`.\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRewriteLinksKeepsTitlesFragmentsAndRefDefs(t *testing.T) {
	body := "[a](a.md#x \"T\") and [r]\n\n[r]: <a.md> 'T'\n"
	got := RewriteLinks(body, func(string) string { return "new dir/a.md" })
	want := "[a](new%20dir/a.md#x \"T\") and [r]\n\n[r]: <new dir/a.md> 'T'\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestNormalizeTargetDecodesAndStaysInTheBundle(t *testing.T) {
	for _, tc := range []struct{ href, from, want string }{
		{"my%20notes/a.md", "", "my notes/a.md"},
		{"/a%2Db.md", "x", "a-b.md"},
		{"../../x.md", "", ""},   // climbs out of the bundle
		{"../x.md", "a", "x.md"}, // stays inside
		{"../../x.md", "a", ""},
	} {
		if got := NormalizeTarget(tc.href, tc.from); got != tc.want {
			t.Errorf("NormalizeTarget(%q, %q) = %q, want %q", tc.href, tc.from, got, tc.want)
		}
	}
}

// The scanner must terminate and not panic on malformed input.
func TestLinkScannerSurvivesMalformedInput(t *testing.T) {
	for _, body := range []string{
		"[", "]", "[a](", "[a](<", "[a](a.md \"unclosed", "```", "`", "\\", "[a]: ", "[]: x.md",
		strings.Repeat("[", 1000), strings.Repeat("`", 999) + "[a](a.md)",
	} {
		_ = ExtractLinkTargets(body)
		_ = RewriteLinks(body, func(string) string { return "x.md" })
	}
}
