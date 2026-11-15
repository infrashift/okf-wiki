package bundle

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func concepts(cs ...*Concept) []*Concept { return cs }

func TestSearchRanksTitleOverTagOverDescriptionOverBody(t *testing.T) {
	cs := concepts(
		&Concept{ID: "body", Title: "B", Body: "the needle is here"},
		&Concept{ID: "desc", Title: "D", Description: "a needle"},
		&Concept{ID: "tag", Title: "T", Tags: []string{"needle"}},
		&Concept{ID: "title", Title: "The needle"},
		&Concept{ID: "none", Title: "Haystack", Body: "nothing"},
	)
	hits := Search(cs, "needle", 0)
	var got []string
	for _, h := range hits {
		got = append(got, h.Concept.ID)
	}
	if want := "title tag desc body"; strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}

func TestSearchIsCaseInsensitiveLiteralSubstring(t *testing.T) {
	cs := concepts(&Concept{ID: "a", Title: "Token Refresh", Body: "Refreshing tokens."})
	for _, q := range []string{"TOKEN", "refresh", "efres", `"token,"`, "(refresh)"} {
		if hits := Search(cs, q, 0); len(hits) != 1 {
			t.Errorf("Search(%q) = %d hits, want 1", q, len(hits))
		}
	}
	// No stemming and no fuzzy match: a term must occur as written.
	for _, q := range []string{"tokenize", "refreshed", "tkn"} {
		if hits := Search(cs, q, 0); len(hits) != 0 {
			t.Errorf("Search(%q) = %d hits, want 0", q, len(hits))
		}
	}
}

func TestSearchLimitAndEmptyQuery(t *testing.T) {
	cs := concepts(
		&Concept{ID: "2", Title: "b", Description: "second\nmore"},
		&Concept{ID: "1", Title: "a"},
		&Concept{ID: "3", Title: "c x"},
	)
	all := Search(cs, "   ", 0)
	if len(all) != 3 || all[0].Concept.ID != "1" || all[1].Concept.ID != "2" {
		t.Fatalf("an empty query lists every concept in title order, got %+v", all)
	}
	if all[1].Score != 0 || all[1].Snippet != "second" {
		t.Errorf("empty-query hit = %+v, want score 0 and the description's first line", all[1])
	}
	if got := Search(cs, "", 2); len(got) != 2 {
		t.Errorf("limit 2 returned %d hits", len(got))
	}
	if got := Search(cs, "", -1); len(got) != 3 {
		t.Errorf("a negative limit returns every hit, got %d", len(got))
	}
	if got := Search(nil, "x", 5); got != nil {
		t.Errorf("no concepts, got %+v", got)
	}
}

// TestSearchOrdersTiesByTitleThenID: equal scores must not depend on the
// order the concepts were loaded in, or two searches could page differently.
func TestSearchOrdersTiesByTitleThenID(t *testing.T) {
	a := &Concept{ID: "a", Title: "Same", Body: "term"}
	b := &Concept{ID: "b", Title: "Same", Body: "term"}
	c := &Concept{ID: "c", Title: "Other", Body: "term"}
	for _, in := range [][]*Concept{{a, b, c}, {c, b, a}, {b, c, a}} {
		hits := Search(in, "term", 0)
		if hits[0].Concept != c || hits[1].Concept != a || hits[2].Concept != b {
			t.Errorf("ties not ordered by title then id: %s %s %s",
				hits[0].Concept.ID, hits[1].Concept.ID, hits[2].Concept.ID)
		}
	}
}

func TestSearchSnippetIsPlainBodyText(t *testing.T) {
	cs := concepts(&Concept{ID: "a", Title: "x", Body: "Use a & b < c, then the needle."})
	hits := Search(cs, "needle", 1)
	if len(hits) != 1 || !strings.Contains(hits[0].Snippet, "a & b < c") {
		t.Errorf("snippet must be raw body text, got %+v", hits)
	}
}

// TestSnippetNeverSplitsARune: the window is cut at byte offsets, and a cut
// through a multi-byte character put invalid UTF-8 in the snippet, which JSON
// encoding then turned into U+FFFD.
func TestSnippetNeverSplitsARune(t *testing.T) {
	for _, filler := range []string{"é", "日本", "🙂", "aé"} {
		body := strings.Repeat(filler, 200) + " needle " + strings.Repeat(filler, 200)
		s := snippetFor(body, []string{"needle"}, "")
		if !utf8.ValidString(s) {
			t.Errorf("filler %q: snippet is not valid UTF-8: %q", filler, s)
		}
		if !strings.Contains(s, "needle") {
			t.Errorf("filler %q: snippet %q lost the match", filler, s)
		}
	}
}

// TestSnippetFindsTheMatchWhereLoweringChangesLength: the match was located in
// strings.ToLower(body) and its offset used to slice body, but lowering changes
// the byte length of some runes. Ⱥ (2 bytes) lowers to ⱥ (3), so the offset ran
// past the end of body and slicing panicked; İ (2 bytes) lowers to i (1), so the
// window landed short of the match.
func TestSnippetFindsTheMatchWhereLoweringChangesLength(t *testing.T) {
	for _, filler := range []string{"Ⱥ", "İ", "K", "\xff"} {
		body := strings.Repeat(filler, 300) + " needle tail"
		var s string
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("filler %q: snippetFor panicked: %v", filler, r)
				}
			}()
			s = snippetFor(body, []string{"needle"}, "")
		}()
		if !strings.Contains(s, "needle") {
			t.Errorf("filler %q: snippet %q does not contain the match", filler, s)
		}
	}
	// Case-insensitive still: the match is found in the original's casing.
	if s := snippetFor("Ⱥ NEEDLE", []string{"needle"}, ""); !strings.Contains(s, "NEEDLE") {
		t.Errorf("snippet %q lost the original casing", s)
	}
}
