package bundle

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/internal/config"
)

// OKF section 11 makes a bundle conformant only if its reserved files follow
// the structure in sections 8 (index.md) and 9 (log.md). These checks report
// what breaks that structure as violations, and what merely departs from the
// section's examples as warnings.

var (
	listItemRe  = regexp.MustCompile(`^ {0,3}[*+-]\s+`)
	linkEntryRe = regexp.MustCompile(`^ {0,3}[*+-]\s+\[.*\]\(\S.*\)`)
	logDateRe   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// CheckIndexFile checks an index.md against OKF v0.3 section 8: no
// frontmatter, except that the bundle-root index may carry the section 8.1
// bundle metadata (okf_version, title, description, license) and extension
// keys; listing entries are links.
func CheckIndexFile(content []byte, isRoot bool) (violations, warnings []string) {
	body := string(content)
	if hasFrontmatter(content) {
		fm, _, rest, err := splitFrontmatter(content)
		switch {
		case err != nil:
			violations = append(violations, "frontmatter is not closed")
		case !isRoot:
			violations = append(violations, "frontmatter is only permitted in the bundle-root index.md")
		default:
			// Section 8.1 also permits extension keys, and section 4.1 says a
			// consumer MUST NOT reject an unknown key; an unknown non-x- key is
			// a warning, so a typo (titel) is still noticed.
			// Section 11.1(1): the block must parse as a YAML mapping. Checked
			// first: an unparseable block used to fall through to the
			// unknown-key warning below, so a broken root index passed.
			var doc yaml.Node
			if perr := yaml.Unmarshal(fm, &doc); perr != nil {
				violations = append(violations, fmt.Sprintf("root index.md frontmatter does not parse as YAML (%v)", perr))
				break
			}
			if len(doc.Content) > 0 && doc.Content[0].Kind != yaml.MappingNode &&
				!(doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null") {
				violations = append(violations, "root index.md frontmatter is not a YAML mapping")
				break
			}
			dups := config.DuplicateKeys(fm)
			if len(dups) > 0 {
				violations = append(violations, "duplicate frontmatter key(s): "+strings.Join(dups, ", "))
			} else if extra := extraKeys(fm, BundleMetaKeys...); len(extra) > 0 {
				var unknown []string
				for _, k := range extra {
					if !strings.HasPrefix(k, "x-") {
						unknown = append(unknown, k)
					}
				}
				if len(unknown) > 0 {
					warnings = append(warnings, fmt.Sprintf(
						"root index.md frontmatter has keys OKF section 8.1 does not define: %s (name your own x-<producer>-<name>)",
						strings.Join(unknown, ", ")))
				}
			}
		}
		if err == nil {
			body = string(rest)
		}
	}

	headings := 0
	inFence := false
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if isFenceLine(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if strings.HasPrefix(line, "#") {
			headings++
		}
		if listItemRe.MatchString(line) && !linkEntryRe.MatchString(line) {
			warnings = append(warnings, fmt.Sprintf("line %d: listing entry is not a link: %s", i+1, strings.TrimSpace(line)))
		}
	}
	if headings == 0 && strings.TrimSpace(body) != "" {
		warnings = append(warnings, "no section heading groups the listing")
	}
	return violations, warnings
}

// CheckLogFile checks a log.md against OKF section 9: date-grouped entries
// under "## YYYY-MM-DD" headings, newest first.
func CheckLogFile(content []byte) (violations []string) {
	if hasFrontmatter(content) {
		violations = append(violations, "log.md carries frontmatter")
	}
	var prev time.Time
	for i, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		d, err := time.Parse("2006-01-02", heading)
		if !logDateRe.MatchString(heading) || err != nil {
			violations = append(violations, fmt.Sprintf("line %d: date heading %q is not an ISO 8601 YYYY-MM-DD date", i+1, heading))
			continue
		}
		if !prev.IsZero() && d.After(prev) {
			violations = append(violations, fmt.Sprintf("line %d: %s follows %s; entries must be newest first",
				i+1, heading, prev.Format("2006-01-02")))
		}
		prev = d
	}
	return violations
}

// DeclaredOKFVersion returns the okf_version a bundle-root index.md declares,
// or "".
func DeclaredOKFVersion(content []byte) string { return config.DeclaredOKFVersion(content) }

func hasFrontmatter(content []byte) bool {
	s := string(content)
	return strings.HasPrefix(s, "---\n") || strings.HasPrefix(s, "---\r\n")
}

func isFenceLine(line string) bool {
	t := strings.TrimLeft(line, " ")
	return len(line)-len(t) <= 3 && (strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"))
}

// extraKeys lists the top-level keys of a frontmatter block other than allowed.
func extraKeys(fm []byte, allowed ...string) []string {
	var m map[string]interface{}
	if err := yaml.Unmarshal(fm, &m); err != nil {
		return []string{"(unparseable)"}
	}
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var extra []string
	for k := range m {
		if !ok[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	return extra
}
