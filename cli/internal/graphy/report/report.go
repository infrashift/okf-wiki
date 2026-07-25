// Package report renders GRAPH_REPORT.md from analysis results.
package report

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"okfw/cli/internal/graphy/analyze"
	"okfw/cli/internal/graphy/model"
)

//go:embed report.md.tmpl
var reportTmpl string

// data wraps the analysis report with rendering extras.
type data struct {
	*analyze.Report
	Suggested []string
}

var funcs = template.FuncMap{
	"conf": func(m map[model.Confidence]int, key string) int { return m[model.Confidence(key)] },
	// markers renders the rationale tally in a stable order — map iteration
	// would otherwise reshuffle the line on every build and churn the report.
	"markers": func(m map[string]int) string {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", m[k], k))
		}
		return strings.Join(parts, ", ")
	},
	"loc": func(n model.Node) string {
		if n.File == "" {
			return ""
		}
		if n.Span.StartLine > 0 {
			return fmt.Sprintf("%s:%d", n.File, n.Span.StartLine)
		}
		return n.File
	},
}

// Render writes the markdown report.
func Render(w io.Writer, r *analyze.Report) error {
	tmpl, err := template.New("report").Funcs(funcs).Parse(reportTmpl)
	if err != nil {
		return err
	}
	return tmpl.Execute(w, data{Report: r, Suggested: suggestions(r)})
}

// suggestions proposes concrete starter queries from the graph's own hubs.
func suggestions(r *analyze.Report) []string {
	var out []string
	if len(r.GodNodes) > 0 {
		out = append(out, fmt.Sprintf("graphy explain %q", r.GodNodes[0].Label))
	}
	if len(r.GodNodes) > 1 {
		out = append(out, fmt.Sprintf("graphy path %q %q", r.GodNodes[0].Label, r.GodNodes[1].Label))
	}
	out = append(out, `graphy query "what are the main subsystems?"`)
	return out
}
