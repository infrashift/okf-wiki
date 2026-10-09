package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"okfw/cli/internal/config"
	"okfw/cli/internal/graph"
)

// normalize and linksIn keep the tests speaking in terms of resolved targets,
// which is what "the link still works" actually means.
func normalize(href, fromDir string) string { return graph.NormalizeTarget(href, fromDir) }

func linksIn(body string) []string { return graph.ExtractLinkTargets(body) }

// --- RelHref ---

func TestRelHref(t *testing.T) {
	for _, tc := range []struct {
		fromDir, target, want string
	}{
		{"", "a.md", "a.md"},
		{"", "dir/a.md", "dir/a.md"},
		{"a", "a/b.md", "b.md"},
		{"a", "b.md", "../b.md"},
		{"a", "c/d.md", "../c/d.md"},
		{"a/b", "a/c.md", "../c.md"},
		{"a/b", "a/b/c.md", "c.md"},
		{"a/b/c", "a/x.md", "../../x.md"},
		{"x", "y/z/deep.md", "../y/z/deep.md"},
	} {
		if got := RelHref(tc.fromDir, tc.target); got != tc.want {
			t.Errorf("RelHref(%q, %q) = %q, want %q", tc.fromDir, tc.target, got, tc.want)
		}
	}
}

// RelHref must round-trip through NormalizeTarget: whatever it produces has to
// resolve back to the same target.
func TestRelHrefRoundTrips(t *testing.T) {
	dirs := []string{"", "a", "a/b", "a/b/c", "x"}
	targets := []string{"top.md", "a/one.md", "a/b/two.md", "z/deep/three.md"}
	for _, d := range dirs {
		for _, target := range targets {
			href := RelHref(d, target)
			if got := normalize(href, d); got != target {
				t.Errorf("RelHref(%q,%q)=%q normalized back to %q", d, target, href, got)
			}
		}
	}
}

// --- ReplaceBody ---

func TestReplaceBodyPreservesFrontmatterBytes(t *testing.T) {
	content := []byte(nestedConcept)
	out, err := ReplaceBody(content, "new body\n")
	if err != nil {
		t.Fatalf("ReplaceBody: %v", err)
	}
	fmEnd := strings.Index(nestedConcept, "---\n\n") + len("---\n")
	if !strings.HasPrefix(string(out), nestedConcept[:fmEnd]) {
		t.Errorf("frontmatter changed:\n%s", out)
	}
	if !strings.HasSuffix(string(out), "new body\n") {
		t.Errorf("body not replaced:\n%s", out)
	}
}

// Round-tripping the body a file already has must be a byte-for-byte no-op —
// including on CRLF files, where rebuilding the block from parsed pieces would
// normalise the delimiters to LF.
func TestReplaceBodyIsIdentityForSameBody(t *testing.T) {
	for _, name := range []string{"lf", "crlf"} {
		t.Run(name, func(t *testing.T) {
			content := []byte("---\nid: \"x\"\ntitle: \"T\"\n---\n\n# T\n\nBody.\n")
			if name == "crlf" {
				content = []byte(strings.ReplaceAll(string(content), "\n", "\r\n"))
			}
			out, err := ReplaceBody(content, config.ExtractBody(content))
			if err != nil {
				t.Fatalf("ReplaceBody: %v", err)
			}
			if string(out) != string(content) {
				t.Errorf("not a no-op:\n%q\nwant:\n%q", out, content)
			}
		})
	}
}

// --- fixtures ---

var conceptSeq int

func concept(t *testing.T, cfg *config.Config, rel, title, body string) string {
	t.Helper()
	conceptSeq++
	id := fmt.Sprintf("019680f0-7e00-7abc-8def-%012x", conceptSeq)
	base := GenerateBasicNote(id, title, "", nil)
	full, err := ReplaceBody([]byte(base), body)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(cfg.RootPath, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, full, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func bodyOf(t *testing.T, p string) string {
	t.Helper()
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return config.ExtractBody(content)
}

// --- MoveConcept ---

func TestMoveConceptRetargetsInboundLinks(t *testing.T) {
	cfg, root := testCfg(t)
	target := concept(t, cfg, "notes/target--0001.md", "target", "# target\n")
	// Three linkers, each using a different href form.
	abs := concept(t, cfg, "notes/abs--0002.md", "abs", "See [T](/notes/target--0001.md).\n")
	relDot := concept(t, cfg, "notes/reldot--0003.md", "reldot", "See [T](./target--0001.md).\n")
	bare := concept(t, cfg, "other/bare--0004.md", "bare", "See [T](../notes/target--0001.md).\n")

	res, err := MoveConcept(cfg, target, filepath.Join(root, "archive", "target--0001.md"))
	if err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}
	if res.FromRel != "notes/target--0001.md" || res.ToRel != "archive/target--0001.md" {
		t.Errorf("result = %+v", res)
	}
	if len(res.Rewrote) != 3 {
		t.Errorf("rewrote %v, want all 3 linkers", res.Rewrote)
	}

	// Bundle-absolute stays absolute.
	if got := bodyOf(t, abs); !strings.Contains(got, "(/archive/target--0001.md)") {
		t.Errorf("absolute link not retargeted: %q", got)
	}
	// Relative stays relative, and keeps its "./" when it still applies.
	if got := bodyOf(t, relDot); !strings.Contains(got, "(../archive/target--0001.md)") {
		t.Errorf("relative link not retargeted: %q", got)
	}
	if got := bodyOf(t, bare); !strings.Contains(got, "(../archive/target--0001.md)") {
		t.Errorf("cross-directory link not retargeted: %q", got)
	}

	// Every rewritten link must resolve to the concept's new home.
	for _, p := range []string{abs, relDot, bare} {
		rel, _ := filepath.Rel(root, p)
		fromDir := RelDir(filepath.ToSlash(rel))
		for _, href := range linksIn(bodyOf(t, p)) {
			resolved := normalize(href, fromDir)
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resolved))); err != nil {
				t.Errorf("%s: link %q resolves to %q which does not exist", rel, href, resolved)
			}
		}
	}
}

// The moved concept's own relative links resolve against its directory, so they
// break when it changes directory. This is the half that fails silently.
func TestMoveConceptFixesItsOwnRelativeLinks(t *testing.T) {
	cfg, root := testCfg(t)
	concept(t, cfg, "notes/sibling--0001.md", "sib", "# sib\n")
	concept(t, cfg, "top--0002.md", "top", "# top\n")
	moving := concept(t, cfg, "notes/moving--0003.md", "moving",
		"Rel [S](./sibling--0001.md), up [T](../top--0002.md), abs [A](/top--0002.md).\n")

	if _, err := MoveConcept(cfg, moving, filepath.Join(root, "deep", "nest", "moving--0003.md")); err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}

	moved := filepath.Join(root, "deep", "nest", "moving--0003.md")
	got := bodyOf(t, moved)

	// Relative links rewritten to still reach the same files from deep/nest/.
	if !strings.Contains(got, "(../../notes/sibling--0001.md)") {
		t.Errorf("sibling link not fixed: %q", got)
	}
	if !strings.Contains(got, "(../../top--0002.md)") {
		t.Errorf("parent-dir link not fixed: %q", got)
	}
	// Bundle-absolute links are position-independent and must be left alone.
	if !strings.Contains(got, "(/top--0002.md)") {
		t.Errorf("absolute link should be untouched: %q", got)
	}

	for _, href := range linksIn(got) {
		resolved := normalize(href, "deep/nest")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resolved))); err != nil {
			t.Errorf("moved concept's link %q resolves to %q which does not exist", href, resolved)
		}
	}
}

func TestMoveConceptRetargetsSelfLink(t *testing.T) {
	cfg, root := testCfg(t)
	self := concept(t, cfg, "notes/self--0001.md", "self", "Me: [self](./self--0001.md).\n")

	res, err := MoveConcept(cfg, self, filepath.Join(root, "archive", "self--0001.md"))
	if err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}
	if !res.SelfLink {
		t.Error("expected SelfLink to be reported")
	}
	got := bodyOf(t, filepath.Join(root, "archive", "self--0001.md"))
	if !strings.Contains(got, "(./self--0001.md)") {
		t.Errorf("self-link should still point at itself: %q", got)
	}
}

func TestMoveConceptKeepsIDAndBumpsTimestamp(t *testing.T) {
	cfg, root := testCfg(t)
	p, id, err := CreateConcept(cfg, Spec{Title: "Subject", Dir: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	zBefore, err := config.ParseAndValidate(before)
	if err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(root, "archive", filepath.Base(p))
	if _, err := MoveConcept(cfg, p, newPath); err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}

	after, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	zAfter, err := config.ParseAndValidate(after)
	if err != nil {
		t.Fatalf("moved concept does not validate: %v", err)
	}
	if zAfter.ID != id {
		t.Errorf("id changed: %q -> %q", id, zAfter.ID)
	}
	if !zAfter.Created.Equal(zBefore.Created) {
		t.Errorf("created changed: %v -> %v", zBefore.Created, zAfter.Created)
	}
	if zAfter.Timestamp.Before(zBefore.Timestamp) {
		t.Errorf("timestamp went backwards: %v -> %v", zBefore.Timestamp, zAfter.Timestamp)
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("the old file still exists")
	}
}

// TestMoveConceptKeepsTheGraphPointingAtTheNewLocation: the link graph is read
// from the files, so a move is reflected the moment the file lands — there is
// no derived copy that could still name the old path.
func TestMoveConceptKeepsTheGraphPointingAtTheNewLocation(t *testing.T) {
	cfg, root := testCfg(t)
	p, id, err := CreateConcept(cfg, Spec{Title: "Findable Subject", Dir: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(root, "archive", filepath.Base(p))
	if _, err := MoveConcept(cfg, p, newPath); err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}

	g, err := LoadGraph(cfg.RootPath)
	if err != nil {
		t.Fatal(err)
	}
	node := g.GetNode(id)
	if node == nil {
		t.Fatalf("concept %s missing from the graph after the move", id)
	}
	if node.FilePath != newPath {
		t.Errorf("graph path = %q, want %q", node.FilePath, newPath)
	}
}

func TestMoveConceptRefreshesIndexMd(t *testing.T) {
	cfg, root := testCfg(t)
	p := concept(t, cfg, "notes/subject--0001.md", "subj", "# subj\n")

	if _, err := MoveConcept(cfg, p, filepath.Join(root, "archive", "subject--0001.md")); err != nil {
		t.Fatalf("MoveConcept: %v", err)
	}
	archiveIdx, err := os.ReadFile(filepath.Join(root, "archive", "index.md"))
	if err != nil {
		t.Fatalf("the new directory should have an index.md: %v", err)
	}
	if !strings.Contains(string(archiveIdx), "subject--0001.md") {
		t.Errorf("archive/index.md should list the concept:\n%s", archiveIdx)
	}
	notesIdx, err := os.ReadFile(filepath.Join(root, "notes", "index.md"))
	if err == nil && strings.Contains(string(notesIdx), "subject--0001.md") {
		t.Errorf("notes/index.md should no longer list it:\n%s", notesIdx)
	}
}

func TestMoveConceptRejections(t *testing.T) {
	cfg, root := testCfg(t)
	p := concept(t, cfg, "notes/a--0001.md", "a", "# a\n")
	concept(t, cfg, "notes/b--0002.md", "b", "# b\n")

	for _, tc := range []struct {
		name string
		dest string
	}{
		{"same place", "notes/a--0001.md"},
		{"onto an existing concept", "notes/b--0002.md"},
		{"a reserved filename", "notes/index.md"},
		{"not a markdown file", "notes/a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := MoveConcept(cfg, p, filepath.Join(root, filepath.FromSlash(tc.dest))); err == nil {
				t.Errorf("moving to %q should fail", tc.dest)
			}
		})
	}
	// The source must survive every rejected attempt.
	if _, err := os.Stat(p); err != nil {
		t.Errorf("source lost after a rejected move: %v", err)
	}
}

func TestMoveConceptRejectsEscapingTheBundle(t *testing.T) {
	cfg, root := testCfg(t)
	p := concept(t, cfg, "a--0001.md", "a", "# a\n")
	outside := filepath.Join(filepath.Dir(root), "escaped.md")
	if _, err := MoveConcept(cfg, p, outside); err == nil {
		t.Error("moving outside the bundle should fail")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("a file was written outside the bundle")
	}
}

// --- DeleteConcept ---

// TestDeleteConceptRemovesTheConcept: with search and the link graph computed
// from the files, deleting the file is the whole operation. This used to have
// to assert against two caches, and a missing prune step is what once left a
// deleted concept in `okfw search` results indefinitely.
func TestDeleteConceptRemovesTheConcept(t *testing.T) {
	cfg, _ := testCfg(t)
	p, id, err := CreateConcept(cfg, Spec{Title: "Doomed Subject"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := DeleteConcept(cfg, p)
	if err != nil {
		t.Fatalf("DeleteConcept: %v", err)
	}
	if res.ID != id {
		t.Errorf("reported id %q, want %q", res.ID, id)
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("the file still exists")
	}
	// A bundle emptied of concepts has no graph to load; either way the
	// deleted id must not be in one.
	if g, err := LoadGraph(cfg.RootPath); err == nil {
		if g.GetNode(id) != nil {
			t.Error("deleted concept still in the graph")
		}
	}
}

// Inbound links are reported so the user knows, but never rewritten: OKF
// requires consumers to tolerate a link whose target does not exist.
func TestDeleteConceptReportsButDoesNotRepairInboundLinks(t *testing.T) {
	cfg, _ := testCfg(t)
	doomed := concept(t, cfg, "notes/doomed--0001.md", "doom", "# doom\n")
	linker := concept(t, cfg, "notes/linker--0002.md", "link", "See [D](/notes/doomed--0001.md).\n")
	before := bodyOf(t, linker)

	res, err := DeleteConcept(cfg, doomed)
	if err != nil {
		t.Fatalf("DeleteConcept: %v", err)
	}
	if len(res.Inbound) != 1 || res.Inbound[0] != "notes/linker--0002.md" {
		t.Errorf("Inbound = %v, want the one linker", res.Inbound)
	}
	if after := bodyOf(t, linker); after != before {
		t.Errorf("the linker's prose was modified:\n%s\nwant:\n%s", after, before)
	}
}

func TestDeleteConceptRefusesReservedFiles(t *testing.T) {
	cfg, root := testCfg(t)
	concept(t, cfg, "a--0001.md", "a", "# a\n")
	if _, err := WriteIndexFiles(cfg, root); err != nil {
		t.Fatal(err)
	}
	if _, err := DeleteConcept(cfg, filepath.Join(root, "index.md")); err == nil {
		t.Error("deleting index.md should be refused")
	}
	if _, err := os.Stat(filepath.Join(root, "index.md")); err != nil {
		t.Errorf("index.md was removed anyway: %v", err)
	}
}

// TestDeleteConceptLeavesNoDerivedState: there is no cache for a delete to
// create or to forget to clean up. This asserts the absence deliberately —
// the bundle after a delete must contain only concepts and listings.
func TestDeleteConceptLeavesNoDerivedState(t *testing.T) {
	cfg, root := testCfg(t)
	p := concept(t, cfg, "a--0001.md", "a", "# a\n")
	if _, err := DeleteConcept(cfg, p); err != nil {
		t.Fatalf("DeleteConcept: %v", err)
	}
	for _, gone := range []string{".index", ".graph.db"} {
		if _, err := os.Stat(filepath.Join(root, gone)); err == nil {
			t.Errorf("%s exists; okfw no longer maintains derived stores", gone)
		}
	}
}

func TestInboundLinksIgnoresSelfAndExternal(t *testing.T) {
	cfg, root := testCfg(t)
	concept(t, cfg, "notes/target--0001.md", "target",
		"I link to myself [me](./target--0001.md) and outward [x](https://example.com/a.md).\n")
	concept(t, cfg, "notes/other--0002.md", "other", "[T](./target--0001.md)\n")

	got, err := InboundLinks(root, "notes/target--0001.md")
	if err != nil {
		t.Fatalf("InboundLinks: %v", err)
	}
	if len(got) != 1 || got[0] != "notes/other--0002.md" {
		t.Errorf("InboundLinks = %v, want just the other concept", got)
	}
}

// A bundle-absolute self-link must follow the concept too. Step 1 skipped every
// "/"-prefixed link and step 2 skips the moved file, so it was left pointing at
// the old path and the move reported self_link: false.
func TestMoveConceptRetargetsAbsoluteSelfLink(t *testing.T) {
	cfg, root := testCfg(t)
	p := concept(t, cfg, "selfy--0001.md", "selfy", "Back to [top](/selfy--0001.md#top).\n")
	dst := filepath.Join(root, "arch", "selfy--0001.md")
	res, err := MoveConcept(cfg, p, dst)
	if err != nil {
		t.Fatal(err)
	}
	if !res.SelfLink {
		t.Error("SelfLink = false")
	}
	if got := bodyOf(t, dst); !strings.Contains(got, "(/arch/selfy--0001.md#top)") {
		t.Errorf("absolute self-link not retargeted: %q", got)
	}
}

// Inbound links in every form the grammar allows are repaired, and link-shaped
// text inside code is not touched.
func TestMoveConceptRepairsEveryLinkFormAndLeavesCodeAlone(t *testing.T) {
	cfg, root := testCfg(t)
	target := concept(t, cfg, "notes/target--0001.md", "target", "# target\n")
	body := "Titled [T](/notes/target--0001.md \"Target\").\n" +
		"Angled [T](<./target--0001.md>).\n" +
		"Encoded [T](/notes/target%2D%2D0001.md).\n" +
		"Ref [T][t].\n\n[t]: ./target--0001.md\n\n" +
		"```md\nExample: [T](/notes/target--0001.md)\n```\n" +
		"Inline `[T](./target--0001.md)` too.\n"
	linker := concept(t, cfg, "notes/linker--0002.md", "linker", body)

	if _, err := MoveConcept(cfg, target, filepath.Join(root, "archive", "target--0001.md")); err != nil {
		t.Fatal(err)
	}
	got := bodyOf(t, linker)
	for _, want := range []string{
		"[T](/archive/target--0001.md \"Target\")",
		"[T](<../archive/target--0001.md>)",
		"[T](/archive/target--0001.md)",
		"[t]: ../archive/target--0001.md",
		"Example: [T](/notes/target--0001.md)",
		"`[T](./target--0001.md)`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
