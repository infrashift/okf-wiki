package bundle

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Hit is one search result.
type Hit struct {
	Concept *Concept
	// Score ranks the hit; higher is better. It is 0 for every hit of an
	// empty query. Compare scores only within one search.
	Score float64
	// Snippet is about 160 bytes of plain body text around the first match,
	// cut on rune boundaries and marked "..." where cut. When the body does not
	// match, it is the first line of the description.
	Snippet string
}

// Search returns the concepts that match query, best first, at most limit of
// them; a limit of 0 or less returns every hit. It is the ranking `okfw
// search` applies, which scans and scores with no index.
//
// The query is split on whitespace into terms, lowercased, with surrounding
// punctuation trimmed. A term matches where it occurs as a literal substring,
// compared case-insensitively, of the title, a tag, the description or the
// body. There is no stemming, no fuzzy match, and no phrase or boolean
// syntax. A concept matching no term is left out. An empty query matches
// every concept, with score 0, in title order.
//
// Ranking weights a match in the title over one in a tag, over the
// description, over the body. Repeated body matches count for less each time,
// and a concept matching every term, or whose title is the query, ranks
// higher. Ties are ordered by title, then ID, so a search over the same
// concepts always returns the same order.
func Search(concepts []*Concept, query string, limit int) []Hit {
	terms := tokenize(query)
	var hits []Hit
	for _, c := range concepts {
		h := Hit{Concept: c}
		if len(terms) > 0 {
			h.Score, h.Snippet = scoreConcept(c, terms)
			if h.Score <= 0 {
				continue
			}
		} else if c.Description != "" {
			h.Snippet = firstLine(c.Description)
		}
		hits = append(hits, h)
	}

	// Highest score first; ties by title then id so paging is stable and two
	// runs over an unchanged bundle agree.
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Concept.Title != b.Concept.Title {
			return a.Concept.Title < b.Concept.Title
		}
		return a.Concept.ID < b.Concept.ID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// Field weights. Title outranks description outranks body because a concept
// whose *title* is the query is almost always the one meant, and okfw's own
// convention is one idea per concept with a one-sentence description.
const (
	weightTitle       = 10.0
	weightTag         = 6.0
	weightDescription = 4.0
	weightBody        = 1.0
	// bonusExactTitle fires when the whole query is the title, which is how an
	// agent following a name from the code graph arrives here.
	bonusExactTitle = 25.0
	// bonusAllTerms rewards a concept matching every term over one matching a
	// single term many times — the property BM25 saturation buys, obtained here
	// by rewarding coverage rather than by damping frequency.
	bonusAllTerms = 8.0
)

// scoreConcept ranks a concept against the query terms and returns a snippet
// around the best body match.
//
// A weighted field match rather than BM25. With one idea per concept and a
// one-sentence description, term frequency within a concept carries far less
// signal than *where* a term appears, and coverage of the query matters more
// than repetition — which is what bonusAllTerms encodes.
func scoreConcept(c *Concept, terms []string) (float64, string) {
	title := strings.ToLower(c.Title)
	desc := strings.ToLower(c.Description)
	lowBody := strings.ToLower(c.Body)
	tags := strings.ToLower(strings.Join(c.Tags, " "))

	var score float64
	hits := 0
	for _, t := range terms {
		var termScore float64
		if strings.Contains(title, t) {
			termScore += weightTitle
		}
		if strings.Contains(tags, t) {
			termScore += weightTag
		}
		if strings.Contains(desc, t) {
			termScore += weightDescription
		}
		if n := strings.Count(lowBody, t); n > 0 {
			// Damp repetition: a term appearing 50 times is not 50x the signal.
			termScore += weightBody * (1 + float64(min(n, 5))/2)
		}
		if termScore > 0 {
			hits++
		}
		score += termScore
	}
	if score == 0 {
		return 0, ""
	}
	if hits == len(terms) {
		score += bonusAllTerms
	}
	if title == strings.Join(terms, " ") {
		score += bonusExactTitle
	}
	return score, snippetFor(c.Body, terms, c.Description)
}

// snippetFor returns a short window of body text around the first matching
// term, falling back to the description.
//
// Plain text, never marked up: the previous implementation used bleve's default
// html highlighter, whose formatter ran html.EscapeString over the text
// *around* each match, so a snippet of a markdown concept came back containing
// `&amp;` and `&lt;`. That was corruption, not markup.
func snippetFor(body string, terms []string, fallback string) string {
	const window = 160
	at := matchOffset(body, terms)
	if at < 0 {
		return firstLine(fallback)
	}
	// Cut on rune boundaries: a byte window through a multi-byte character put
	// invalid UTF-8 in the snippet, which JSON encoding turned into U+FFFD.
	start := max(at-window/2, 0)
	for start > 0 && !utf8.RuneStart(body[start]) {
		start--
	}
	end := min(start+window, len(body))
	for end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	s := strings.TrimSpace(strings.ReplaceAll(body[start:end], "\n", " "))
	if start > 0 {
		s = "..." + s
	}
	if end < len(body) {
		s += "..."
	}
	return s
}

// matchOffset returns the byte offset in body of the earliest term, matched
// case-insensitively, or -1.
//
// The match is found in strings.ToLower(body), but that offset cannot slice
// body: lowering changes the byte length of some runes — Ⱥ grows from two bytes
// to three, İ and the Kelvin sign shrink to one, an invalid byte becomes a
// three-byte U+FFFD — so it pointed elsewhere in the original, and past its end
// in a body dense with growing runes, which panicked the whole search. The
// offset is translated back by re-walking body and summing each rune's lowered
// width, the same per-rune mapping strings.ToLower applies.
func matchOffset(body string, terms []string) int {
	low := strings.ToLower(body)
	idx := -1
	for _, t := range terms {
		if i := strings.Index(low, t); i >= 0 && (idx < 0 || i < idx) {
			idx = i
		}
	}
	if idx < 0 {
		return -1
	}
	n := 0
	for i, r := range body {
		if n >= idx {
			return i
		}
		n += utf8.RuneLen(unicode.ToLower(r))
	}
	return len(body)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// tokenize lowercases and splits a query on whitespace, dropping punctuation
// that would never match inside a word.
func tokenize(q string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(q)) {
		f = strings.Trim(f, `.,;:!?"'()[]{}`)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
