package extract

import (
	"strings"
	"testing"

	"okfw/cli/internal/graphy/detect"
	"okfw/cli/internal/graphy/model"
)

func testMarkers() map[string]bool {
	return DefaultRationaleOptions().markerSet()
}

// found returns every marker+summary the scanner produces for a source.
func found(t *testing.T, lang, src string) []string {
	t.Helper()
	var out []string
	for _, b := range scanComments(lang, []byte(src)) {
		for _, r := range b.rationales(testMarkers()) {
			out = append(out, r.marker+"|"+r.summary)
		}
	}
	return out
}

// TestScanCommentsFalsePositives is the table that earns the literal-aware
// scanner over a plain line scanner: every one of these markers is data, not
// a comment, and a naive scan would turn each into a bogus rationale node.
func TestScanCommentsFalsePositives(t *testing.T) {
	for _, tc := range []struct {
		name, lang, src string
	}{
		{"go interpreted string", "go", "package p\nfunc f() { msg := \"TODO: nope\"; _ = msg }\n"},
		{"go raw string", "go", "package p\nvar s = `WHY: nope`\n"},
		{"go rune-ish quote", "go", "package p\nvar s = \"it's a HACK: nope\"\n"},
		{"sql single quotes", "sql", "INSERT INTO t VALUES ('HACK: nope');\n"},
		{"yaml quoted scalar", "yaml", "key: \"# TODO: nope\"\nother: 'FIXME: nope'\n"},
		{"python triple quote", "python", "s = \"\"\"\nWHY: nope\n\"\"\"\n"},
		{"url fragment", "go", "package p\n// see http://example.com/#TODO\n"},
		{"mid-sentence marker", "go", "package p\n// see the TODO: below\n"},
		{"bare marker no text", "go", "package p\n// TODO:\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := found(t, tc.lang, tc.src); len(got) != 0 {
				t.Errorf("expected no rationale, got %v", got)
			}
		})
	}
}

func TestScanCommentsPrefixesAndContinuation(t *testing.T) {
	for _, tc := range []struct {
		name, lang, src string
		want            []string
	}{
		{"go line", "go", "package p\n// WHY: lock ordering matters\n", []string{"WHY|lock ordering matters"}},
		{"go block", "go", "package p\n/*\nDECISION: use bbolt\nit is embedded\n*/\n",
			[]string{"DECISION|use bbolt"}},
		{"yaml hash", "yaml", "# HACK: the API needs a trailing slash\nkey: 1\n",
			[]string{"HACK|the API needs a trailing slash"}},
		{"sql dashes", "sql", "-- WHY: denormalized for read speed\nSELECT 1;\n",
			[]string{"WHY|denormalized for read speed"}},
		{"terraform hash and slash", "terraform", "# TODO: pin the version\n// FIXME: split this module\n",
			[]string{"TODO|pin the version", "FIXME|split this module"}},
		{"marker below a doc line", "go", "package p\n// Helper does a thing.\n// WHY: callers rely on the order\n",
			[]string{"WHY|callers rely on the order"}},
		{"two markers one block", "go", "package p\n// WHY: first reason\n// TODO: second thing\n",
			[]string{"WHY|first reason", "TODO|second thing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := found(t, tc.lang, tc.src)
			if strings.Join(got, ";") != strings.Join(tc.want, ";") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestScanCommentsContinuationJoinsLines(t *testing.T) {
	src := "package p\n// WHY: the first reason\n// and the second line\n"
	blocks := scanComments("go", []byte(src))
	rs := blocks[0].rationales(testMarkers())
	if len(rs) != 1 {
		t.Fatalf("got %d rationales, want 1", len(rs))
	}
	if rs[0].body != "the first reason\nand the second line" {
		t.Errorf("body = %q", rs[0].body)
	}
	if rs[0].summary != "the first reason" {
		t.Errorf("summary = %q, want the first line only", rs[0].summary)
	}
	if rs[0].startLine != 2 || rs[0].endLine != 3 {
		t.Errorf("span = %d..%d, want 2..3", rs[0].startLine, rs[0].endLine)
	}
}

func TestScanCommentsRespectsPerFileCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n")
	for i := 0; i < maxRationalePerFile+20; i++ {
		// Blank line between each so they form separate blocks.
		b.WriteString("// TODO: item\n\n")
	}
	file := detect.File{Path: "p.go", Language: "go"}
	res := &Result{Nodes: []model.Node{{ID: "go:file:p.go#", Kind: model.KindFile}}}
	attachRationale(file, []byte(b.String()), res, DefaultRationaleOptions())

	got := 0
	for _, n := range res.Nodes {
		if n.Kind == model.KindRationale {
			got++
		}
	}
	if got != maxRationalePerFile {
		t.Errorf("emitted %d rationale nodes, want the cap of %d", got, maxRationalePerFile)
	}
}

func TestRationaleOwnerAttachment(t *testing.T) {
	fileNode := model.Node{ID: "go:file:p.go#", Kind: model.KindFile, Span: model.Span{StartLine: 1}}
	fn := model.Node{ID: "go:function:p.go#Helper", Kind: model.KindFunction,
		Span: model.Span{StartLine: 10, EndLine: 20}}
	inner := model.Node{ID: "go:function:p.go#Inner", Kind: model.KindFunction,
		Span: model.Span{StartLine: 12, EndLine: 14}}
	noEnd := model.Node{ID: "go:config:p.go#key", Kind: model.KindStruct,
		Span: model.Span{StartLine: 30}} // EndLine unset, as several pure-Go extractors leave it

	cands := ownerCandidates([]model.Node{fileNode, fn, inner, noEnd})

	t.Run("next definition below", func(t *testing.T) {
		// Comment ends at line 9, Helper starts at 10.
		if got := ownerFor(cands, 8, 9); got < 0 || cands[got].ID != fn.ID {
			t.Errorf("owner = %v, want Helper", ownerIDAt(cands, got))
		}
	})
	t.Run("too far above is not adopted", func(t *testing.T) {
		// Comment ends at line 2; Helper at 10 is well beyond maxOwnerGap, and
		// nothing encloses line 1, so this falls through to the file node.
		if got := ownerFor(cands, 1, 2); got >= 0 {
			t.Errorf("owner = %v, want the file-node fallback", ownerIDAt(cands, got))
		}
	})
	t.Run("smallest enclosing span", func(t *testing.T) {
		// Line 13 is inside both Helper (10..20) and Inner (12..14).
		if got := ownerFor(cands, 13, 13); got < 0 || cands[got].ID != inner.ID {
			t.Errorf("owner = %v, want Inner (the tighter span)", ownerIDAt(cands, got))
		}
	})
	t.Run("node without EndLine cannot enclose", func(t *testing.T) {
		if got := ownerFor(cands, 35, 35); got >= 0 {
			t.Errorf("owner = %v, want no owner: the candidate has no EndLine", ownerIDAt(cands, got))
		}
	})
}

func ownerIDAt(cands []model.Node, i int) string {
	if i < 0 {
		return "<file>"
	}
	return cands[i].ID
}

// TestRationaleIDStability is the core identity guarantee: rationale node ids
// must not move when unrelated code shifts the file, or every edit would churn
// the exported concepts and discard their okfw ids and creation dates.
func TestRationaleIDStability(t *testing.T) {
	body := "package p\n\n// WHY: callers depend on the ordering\nfunc Helper() {}\n"
	shifted := "package p\n" + strings.Repeat("\n", 20) + "\n// WHY: callers depend on the ordering\nfunc Helper() {}\n"

	ids := func(src string) []string {
		file := detect.File{Path: "p.go", Language: "go"}
		res := &Result{Nodes: []model.Node{
			{ID: "go:file:p.go#", Kind: model.KindFile, Span: model.Span{StartLine: 1}},
			{ID: "go:function:p.go#Helper", Kind: model.KindFunction,
				Span: model.Span{StartLine: lineOf(src, "func Helper"), EndLine: lineOf(src, "func Helper")}},
		}}
		attachRationale(file, []byte(src), res, DefaultRationaleOptions())
		var out []string
		for _, n := range res.Nodes {
			if n.Kind == model.KindRationale {
				out = append(out, n.ID)
			}
		}
		return out
	}

	a, b := ids(body), ids(shifted)
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("expected one rationale each, got %v and %v", a, b)
	}
	if a[0] != b[0] {
		t.Errorf("id changed when the file shifted: %q vs %q", a[0], b[0])
	}
	if !strings.Contains(a[0], "Helper@WHY@0") {
		t.Errorf("id %q should key on the owner, marker and ordinal", a[0])
	}
}

func lineOf(src, needle string) int {
	for i, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, needle) {
			return i + 1
		}
	}
	return 0
}

// TestRationalePreservesFileNodeFirst guards the invariant every extractor and
// the build's resolution pass depend on.
func TestRationalePreservesFileNodeFirst(t *testing.T) {
	sources := map[string]string{
		"go":         "package p\n// WHY: reason\nfunc F() {}\n",
		"yaml":       "# WHY: reason\nkey: value\n",
		"sql":        "-- WHY: reason\nCREATE TABLE t (id INT);\n",
		"terraform":  "# WHY: reason\nresource \"aws_s3_bucket\" \"b\" {}\n",
		"python":     "# WHY: reason\ndef f():\n    pass\n",
		"javascript": "// WHY: reason\nfunction f() {}\n",
		"typescript": "// WHY: reason\nfunction f(): void {}\n",
		"json":       "{\"a\": 1}\n",
		"cue":        "// WHY: reason\n#Schema: {a: int}\n",
		"markdown":   "# Title\n\nSome text.\n",
	}
	for lang, src := range sources {
		t.Run(lang, func(t *testing.T) {
			session, err := NewSession([]string{lang})
			if err != nil {
				t.Fatalf("NewSession: %v", err)
			}
			defer session.Close()
			file := detect.File{Path: "fixture." + lang, Language: lang}
			res, err := session.Extract(file, []byte(src))
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if len(res.Nodes) == 0 {
				t.Fatal("no nodes")
			}
			if k := res.Nodes[0].Kind; k != model.KindFile && k != model.KindDoc {
				t.Errorf("Nodes[0].Kind = %q, want the file-level node", k)
			}
		})
	}
}

func TestIsADR(t *testing.T) {
	for _, tc := range []struct {
		name, path, src string
		want            bool
	}{
		{"adr directory", "docs/adr/0001-use-bbolt.md", "# Use bbolt\n", true},
		{"decisions directory", "decisions/pick-a-db.md", "# Pick\n", true},
		{"numbered filename", "notes/0007-choose-cue.md", "# Choose\n", true},
		{"adr-prefixed filename", "notes/ADR-0007-choose-cue.md", "# Choose\n", true},
		{"frontmatter status", "docs/thing.md", "---\nstatus: accepted\n---\n\n# Thing\n", true},
		{"plain doc", "docs/architecture.md", "# Architecture\n", false},
		{"okfw todo is not an ADR", "docs/task.md", "---\nstatus: open\n---\n\n# Task\n", false},
		{"bundle concept excluded", ".okfw/adr/0001-thing.md", "# Thing\n", false},
		{"graphy dir excluded", ".graphy/adr/0001-thing.md", "# Thing\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isADR(tc.path, []byte(tc.src), DefaultADRDirs); got != tc.want {
				t.Errorf("isADR(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestADRRefsAndContent(t *testing.T) {
	src := `# Use bbolt for the graph store

## Context

We evaluated ` + "`Neo4j`" + ` and rejected it.

## Decision

Use ` + "`bbolt`" + `, driven through ` + "`Store.PutFile`" + `.
See [the store](../../cli/store.go). Ignore ` + "`the`" + ` and ` + "`ab`" + `.
`
	file := detect.File{Path: "docs/adr/0001-use-bbolt.md", Language: "markdown"}
	res := &Result{Nodes: []model.Node{{ID: "markdown:doc:docs/adr/0001-use-bbolt.md#", Kind: model.KindDoc}}}
	attachRationale(file, []byte(src), res, DefaultRationaleOptions())

	var rat *model.Node
	for i := range res.Nodes {
		if res.Nodes[i].Kind == model.KindRationale {
			rat = &res.Nodes[i]
		}
	}
	if rat == nil {
		t.Fatal("no ADR rationale node emitted")
	}
	if rat.Label != "ADR: Use bbolt for the graph store" {
		t.Errorf("label = %q", rat.Label)
	}
	if !strings.Contains(rat.Body, "Use `bbolt`") {
		t.Errorf("body missing the decision: %q", rat.Body)
	}
	if !strings.Contains(rat.Body, "Neo4j") {
		t.Errorf("body missing the context: %q", rat.Body)
	}

	names := map[string]bool{}
	for _, r := range res.Refs {
		if r.Relation != model.RelExplains {
			t.Errorf("ref %q has relation %q, want explains", r.Name, r.Relation)
		}
		names[r.Name] = true
	}
	for _, want := range []string{"Neo4j", "bbolt", "Store.PutFile", "cli/store.go"} {
		if !names[want] {
			t.Errorf("missing ref %q (got %v)", want, names)
		}
	}
	if names["the"] {
		t.Error("stoplist word leaked into refs")
	}
	if names["ab"] {
		t.Error("too-short identifier leaked into refs")
	}
}

// TestADRNodeIsAdditive: the markdown extractor's own doc and heading nodes
// must survive, so existing goldens and doc→code link resolution are untouched.
func TestADRNodeIsAdditive(t *testing.T) {
	src := "# Use bbolt\n\n## Decision\n\nUse it.\n"
	session, err := NewSession([]string{"markdown"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	file := detect.File{Path: "docs/adr/0001-use-bbolt.md", Language: "markdown"}
	res, err := session.Extract(file, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[model.Kind]int{}
	for _, n := range res.Nodes {
		kinds[n.Kind]++
	}
	if kinds[model.KindDoc] != 1 {
		t.Errorf("doc nodes = %d, want 1", kinds[model.KindDoc])
	}
	if kinds[model.KindHeading] == 0 {
		t.Error("heading nodes were dropped")
	}
	if kinds[model.KindRationale] != 1 {
		t.Errorf("rationale nodes = %d, want 1", kinds[model.KindRationale])
	}
}
