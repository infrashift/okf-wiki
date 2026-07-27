package index

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
)

// ConceptDoc is the structure we feed into the index
type ConceptDoc struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`                  // OKF free-form concept kind
	Kind        string   `json:"kind"`                  // structured: "note", "daily-note", "issue"
	Description string   `json:"description,omitempty"` // OKF one-sentence summary
	Project     string   `json:"project"`
	Tags        []string `json:"tags"`
	Body        string   `json:"body"`
	FilePath    string   `json:"file_path"`
	Created     string   `json:"created"`
	Timestamp   string   `json:"timestamp,omitempty"` // last meaningful change

	// OKF lifecycle (section 5.4-5.5)
	Status     string `json:"status,omitempty"`      // draft | stable | deprecated
	StaleAfter string `json:"stale_after,omitempty"` // YYYY-MM-DD
}

// SearchResult represents a single search hit
type SearchResult struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Type     string   `json:"type"`
	Kind     string   `json:"kind"`
	Project  string   `json:"project"`
	Tags     []string `json:"tags"`
	FilePath string   `json:"file_path"`
	Score    float64  `json:"score"`
	Snippet  string   `json:"snippet,omitempty"`

	// OKF lifecycle
	Status     string `json:"status,omitempty"`
	StaleAfter string `json:"stale_after,omitempty"`
}

// SearchOptions configures search behavior
type SearchOptions struct {
	Query   string   // Full-text query
	Project string   // Filter by project
	Type    string   // Filter by OKF type (free-form)
	Kind    string   // Filter by structured kind (note/daily-note/issue)
	Tags    []string // Filter by tags (AND)
	Limit   int      // Max results (default 20)
	Offset  int      // Pagination offset

	// OKF lifecycle filters
	Status string // Filter by lifecycle status (draft/stable/deprecated)
	Stale  bool   // Only concepts whose stale_after has passed
}

func CreateMapping() mapping.IndexMapping {
	indexMapping := bleve.NewIndexMapping()

	// Full-text fields with English stemming
	textMapping := bleve.NewTextFieldMapping()
	textMapping.Analyzer = "en"
	indexMapping.DefaultMapping.AddFieldMappingsAt("body", textMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("title", textMapping)

	// Keyword fields (exact match, no tokenization)
	keywordMapping := bleve.NewTextFieldMapping()
	keywordMapping.Analyzer = "keyword"
	indexMapping.DefaultMapping.AddFieldMappingsAt("id", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("type", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("kind", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("project", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("tags", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("file_path", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("created", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("timestamp", keywordMapping)

	// Description is full-text searchable
	indexMapping.DefaultMapping.AddFieldMappingsAt("description", textMapping)

	// OKF lifecycle keyword fields
	indexMapping.DefaultMapping.AddFieldMappingsAt("status", keywordMapping)
	indexMapping.DefaultMapping.AddFieldMappingsAt("stale_after", keywordMapping)

	return indexMapping
}

func OpenOrCreateIndex(path string) (bleve.Index, error) {
	idx, err := bleve.Open(path)
	if err == bleve.ErrorIndexPathDoesNotExist {
		return bleve.New(path, CreateMapping())
	}
	return idx, err
}

// IndexDocument adds or updates a document in the index
func IndexDocument(idx bleve.Index, doc *ConceptDoc) error {
	return idx.Index(doc.ID, doc)
}

// DeleteDocument removes a document from the index
func DeleteDocument(idx bleve.Index, id string) error {
	return idx.Delete(id)
}

// AllDocIDs returns every document id currently in the index. Used to find
// indexed concepts whose file no longer exists, since nothing else in the
// index knows a document has become stale.
func AllDocIDs(idx bleve.Index) ([]string, error) {
	count, err := idx.DocCount()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	req := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
	req.Size = int(count)
	res, err := idx.Search(req)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.Hits))
	for _, hit := range res.Hits {
		ids = append(ids, hit.ID)
	}
	return ids, nil
}

// Search performs a search with the given options
func Search(idx bleve.Index, opts SearchOptions) ([]SearchResult, int, error) {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	// Build the query
	var queries []query.Query

	// Full-text query on body and title
	if opts.Query != "" {
		// Search across title and body
		titleQuery := bleve.NewMatchQuery(opts.Query)
		titleQuery.SetField("title")
		titleQuery.SetBoost(2.0) // Boost title matches

		bodyQuery := bleve.NewMatchQuery(opts.Query)
		bodyQuery.SetField("body")

		textQuery := bleve.NewDisjunctionQuery(titleQuery, bodyQuery)
		queries = append(queries, textQuery)
	}

	// Project filter
	if opts.Project != "" {
		projectQuery := bleve.NewTermQuery(opts.Project)
		projectQuery.SetField("project")
		queries = append(queries, projectQuery)
	}

	// Tags filter (AND - must have all tags)
	for _, tag := range opts.Tags {
		tagQuery := bleve.NewTermQuery(tag)
		tagQuery.SetField("tags")
		queries = append(queries, tagQuery)
	}

	// Type filter (OKF free-form)
	if opts.Type != "" {
		typeQuery := bleve.NewTermQuery(opts.Type)
		typeQuery.SetField("type")
		queries = append(queries, typeQuery)
	}

	// Kind filter (structured)
	if opts.Kind != "" {
		kindQuery := bleve.NewTermQuery(opts.Kind)
		kindQuery.SetField("kind")
		queries = append(queries, kindQuery)
	}

	// Lifecycle status filter
	if opts.Status != "" {
		statusQuery := bleve.NewTermQuery(opts.Status)
		statusQuery.SetField("status")
		queries = append(queries, statusQuery)
	}

	// Stale filter: stale_after is an absolute date, so "stale" is everything
	// with a stale_after at or before today (OKF section 5.5, inclusive).
	if opts.Stale {
		today := time.Now().Format("2006-01-02")
		staleQuery := bleve.NewTermRangeInclusiveQuery("0000-00-00", today, &[]bool{true}[0], &[]bool{true}[0])
		staleQuery.SetField("stale_after")
		queries = append(queries, staleQuery)
	}

	// Combine all queries with AND
	var finalQuery query.Query
	if len(queries) == 0 {
		finalQuery = bleve.NewMatchAllQuery()
	} else if len(queries) == 1 {
		finalQuery = queries[0]
	} else {
		finalQuery = bleve.NewConjunctionQuery(queries...)
	}

	// Create search request
	searchRequest := bleve.NewSearchRequest(finalQuery)
	searchRequest.Size = opts.Limit
	searchRequest.From = opts.Offset
	searchRequest.Fields = []string{"id", "title", "type", "kind", "project", "tags", "file_path", "status", "stale_after"}

	// Add highlighting for body matches
	if opts.Query != "" {
		searchRequest.Highlight = bleve.NewHighlight()
	}

	// Execute search
	searchResult, err := idx.Search(searchRequest)
	if err != nil {
		return nil, 0, fmt.Errorf("search failed: %w", err)
	}

	// Convert results
	results := make([]SearchResult, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		result := SearchResult{
			ID:    hit.ID,
			Score: hit.Score,
		}

		// Extract fields
		if title, ok := hit.Fields["title"].(string); ok {
			result.Title = title
		}
		if typ, ok := hit.Fields["type"].(string); ok {
			result.Type = typ
		}
		if kind, ok := hit.Fields["kind"].(string); ok {
			result.Kind = kind
		}
		if project, ok := hit.Fields["project"].(string); ok {
			result.Project = project
		}
		if filePath, ok := hit.Fields["file_path"].(string); ok {
			result.FilePath = filePath
		}
		if tags, ok := hit.Fields["tags"].([]interface{}); ok {
			for _, t := range tags {
				if s, ok := t.(string); ok {
					result.Tags = append(result.Tags, s)
				}
			}
		}
		// Extract OKF lifecycle fields
		if status, ok := hit.Fields["status"].(string); ok {
			result.Status = status
		}
		if staleAfter, ok := hit.Fields["stale_after"].(string); ok {
			result.StaleAfter = staleAfter
		}

		// Extract highlight snippet
		if len(hit.Fragments) > 0 {
			for _, fragments := range hit.Fragments {
				if len(fragments) > 0 {
					result.Snippet = fragments[0]
					break
				}
			}
		}

		results = append(results, result)
	}

	return results, int(searchResult.Total), nil
}

// SearchResultsToJSON converts search results to JSON string
func SearchResultsToJSON(results []SearchResult) (string, error) {
	data, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatSearchResults formats results for terminal display
func FormatSearchResults(results []SearchResult, total int) string {
	if len(results) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results:\n\n", total))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   ID: %s", r.ID))
		if r.Project != "" {
			sb.WriteString(fmt.Sprintf(" | Project: %s", r.Project))
		}
		// Absent status means stable (OKF section 5.4), so only call out the
		// states worth noticing.
		if r.Status == "draft" || r.Status == "deprecated" {
			sb.WriteString(fmt.Sprintf(" | Status: %s", r.Status))
		}
		if r.IsStale(time.Now()) {
			sb.WriteString(fmt.Sprintf(" | STALE since %s", r.StaleAfter))
		}
		sb.WriteString("\n")
		if len(r.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("   Tags: %s\n", strings.Join(r.Tags, ", ")))
		}
		if r.FilePath != "" {
			sb.WriteString(fmt.Sprintf("   File: %s\n", r.FilePath))
		}
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   ...%s...\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// IsStale reports whether the hit's stale_after has passed, inclusive of the
// boundary day (OKF section 5.5).
func (r SearchResult) IsStale(today time.Time) bool {
	if r.StaleAfter == "" {
		return false
	}
	cutoff, err := time.Parse("2006-01-02", r.StaleAfter)
	if err != nil {
		return false
	}
	y, m, d := today.Date()
	return !time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Before(cutoff)
}
