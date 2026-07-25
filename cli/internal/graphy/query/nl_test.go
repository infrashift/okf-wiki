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
