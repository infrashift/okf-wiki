package query

import (
	"context"
	"errors"
	"strings"
	"testing"

	"okfw/cli/internal/graphy/llm"
)

func TestNLWithLLMSeeds(t *testing.T) {
	s := fixture(t)
	fake := &llm.Fake{Responses: []string{`{"node_ids": ["A"], "depth": 1}`}}

	res, err := NL(context.Background(), s, fake, "how does auth work?", NLOptions{})
	if err != nil {
		t.Fatalf("NL: %v", err)
	}
	if res.Mode != "llm:fake" {
		t.Errorf("mode = %q", res.Mode)
	}
	got := map[string]bool{}
	for _, n := range res.Nodes {
		got[n.ID] = true
	}
	// A plus its depth-1 neighborhood (B, DOC).
	for _, want := range []string{"A", "B", "DOC"} {
		if !got[want] {
			t.Errorf("missing %s: %v", want, got)
		}
	}
	if len(fake.Calls) != 1 {
		t.Errorf("calls = %d, want 1", len(fake.Calls))
	}
	if !strings.Contains(fake.Calls[0], "how does auth work?") || !strings.Contains(fake.Calls[0], "Alpha") {
		t.Errorf("prompt lacks question or index:\n%s", fake.Calls[0])
	}
}

func TestNLFencedJSONAndUnknownIDs(t *testing.T) {
	s := fixture(t)
	fake := &llm.Fake{Responses: []string{
		"Here is my plan:\n```json\n{\"node_ids\": [\"A\", \"NOPE\"], \"depth\": 1}\n```",
	}}
	res, err := NL(context.Background(), s, fake, "auth", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "llm:fake" || len(res.Seeds) != 1 || res.Seeds[0] != "A" {
		t.Errorf("res = %+v", res)
	}
}

func TestNLRetriesThenFallsBack(t *testing.T) {
	s := fixture(t)
	fake := &llm.Fake{Responses: []string{"not json at all", "still not json"}}

	res, err := NL(context.Background(), s, fake, "auth", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", len(fake.Calls))
	}
	if !strings.HasPrefix(res.Mode, "keyword") {
		t.Errorf("mode = %q, want keyword fallback", res.Mode)
	}
	if len(res.Nodes) == 0 {
		t.Errorf("fallback returned nothing")
	}
}

func TestNLBackendErrorFallsBack(t *testing.T) {
	s := fixture(t)
	fake := &llm.Fake{Err: errors.New("boom")}
	res, err := NL(context.Background(), s, fake, "auth", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Mode, "keyword") || !strings.Contains(res.Mode, "boom") {
		t.Errorf("mode = %q", res.Mode)
	}
}

func TestNLWithoutBackend(t *testing.T) {
	s := fixture(t)
	res, err := NL(context.Background(), s, nil, "auth", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != "keyword" {
		t.Errorf("mode = %q", res.Mode)
	}
	got := map[string]bool{}
	for _, n := range res.Nodes {
		got[n.ID] = true
	}
	if !got["A"] || !got["DOC"] {
		t.Errorf("keyword mode missing hits: %v", got)
	}
}

// TestNLIsCappedAndSaysSo is the test whose absence was the defect. Depth
// bounded the expansion, nothing bounded the result, and graphy_query — the
// tool whose description tells agents to prefer it over reading files —
// returned 199KB (~50k tokens) at depth 2 on this repository's own graph, with
// no truncation flag because there was nothing to truncate against.
func TestNLIsCappedAndSaysSo(t *testing.T) {
	s := hubFixture(t, 120)

	res, err := NL(context.Background(), s, nil, "hub", NLOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != 10 {
		t.Errorf("nodes = %d, want the requested limit of 10", len(res.Nodes))
	}
	if !res.More {
		t.Error("truncated result did not announce itself")
	}
	if res.Total <= 10 {
		t.Errorf("total = %d, want the pre-limit count", res.Total)
	}
	// Every edge must be induced over the nodes that survived, or the MCP
	// layer's index encoding has no endpoint to point at.
	kept := map[string]bool{}
	for _, n := range res.Nodes {
		kept[n.ID] = true
	}
	for _, e := range res.Edges {
		if !kept[e.Source] || !kept[e.Target] {
			t.Errorf("edge %s->%s escapes the node cap", e.Source, e.Target)
		}
	}
}

func TestNLDefaultsToDefaultLimit(t *testing.T) {
	s := hubFixture(t, 120)
	res, err := NL(context.Background(), s, nil, "hub", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) != DefaultLimit {
		t.Errorf("nodes = %d, want DefaultLimit %d", len(res.Nodes), DefaultLimit)
	}
}

// TestNLExcludesTestEntitiesUnlessAsked pins the default that is worth 37% of
// the payload on its own: a well-tested hub's neighbourhood is mostly its own
// suite, which crowds out what the question was about.
func TestNLExcludesTestEntitiesUnlessAsked(t *testing.T) {
	s := hubFixture(t, 20)

	res, err := NL(context.Background(), s, nil, "hub", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes {
		if n.Test {
			t.Errorf("test entity %s reached the default result", n.ID)
		}
	}

	withTests, err := NL(context.Background(), s, nil, "hub", NLOptions{IncludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withTests.Nodes) <= len(res.Nodes) {
		t.Errorf("tests:true returned %d nodes, not more than the default %d",
			len(withTests.Nodes), len(res.Nodes))
	}
}

// TestKeywordSeedsSkipTestEntities: Neighbors exempts seeds from the test
// filter so that naming a test function still answers. A keyword-scored seed
// was never named, and test helpers routinely own the best-matching label —
// "how does the MCP server work" seeded on mcpTestServer on this repo.
func TestKeywordSeedsSkipTestEntities(t *testing.T) {
	s := testSeedFixture(t)

	res, err := NL(context.Background(), s, nil, "widget", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Nodes {
		if n.Test {
			t.Errorf("keyword seeding reached test entity %s", n.ID)
		}
	}
	if len(res.Nodes) == 0 {
		t.Error("filtering the seeds emptied the answer")
	}

	withTests, err := NL(context.Background(), s, nil, "widget", NLOptions{IncludeTests: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(withTests.Nodes) <= len(res.Nodes) {
		t.Errorf("tests:true returned %d nodes, not more than %d", len(withTests.Nodes), len(res.Nodes))
	}
}

// TestKeywordSeedsFallBackToTestsWhenThatIsAll: a question genuinely about the
// suite must answer from the suite rather than return an empty graph.
func TestKeywordSeedsFallBackToTestsWhenThatIsAll(t *testing.T) {
	s := testSeedFixture(t)
	res, err := NL(context.Background(), s, nil, "widgetTestHelper", NLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Nodes) == 0 {
		t.Error("a question with only test-file matches returned nothing")
	}
}
