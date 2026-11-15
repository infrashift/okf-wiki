package config

import (
	"fmt"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// OKF section 5: every timestamp-valued key is an ISO 8601 datetime with an
// explicit UTC offset. That covers generated.at, verified[].at, stale_after,
// sources[].last_modified and both ends of usage_window — and okfw holds its
// own created/timestamp to the same form.
//
// A bare date is not accepted on read. "2026-12-31" names a different instant
// in every timezone, so the same bundle could be stale in one office and fresh
// in another; the spec moved off dates for exactly that reason, and a
// midnight-UTC fallback would quietly put the disagreement back. Writers are
// more forgiving (see NormalizeDateTime), because there a human typed the value
// and okfw can say what it turned it into.
var dateTimeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

var dateOnlyRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ParseDateTime parses an OKF timestamp, rejecting a bare date or a datetime
// without an offset.
func ParseDateTime(s string) (time.Time, error) {
	if !dateTimeRe.MatchString(s) {
		return time.Time{}, fmt.Errorf("%q is not an ISO 8601 datetime with a UTC offset (e.g. 2026-12-31T00:00:00Z)", s)
	}
	return time.Parse(time.RFC3339Nano, s)
}

// IsDateOnly reports whether s is a bare YYYY-MM-DD date — the form OKF used
// for these fields before it required datetimes.
func IsDateOnly(s string) bool { return dateOnlyRe.MatchString(s) }

// NormalizeDateTime turns writer input into the OKF form: a datetime with an
// offset is kept exactly as written, and a bare date becomes midnight UTC.
func NormalizeDateTime(s string) (string, error) {
	if IsDateOnly(s) {
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "", fmt.Errorf("%q is not a valid date", s)
		}
		return s + "T00:00:00Z", nil
	}
	if _, err := ParseDateTime(s); err != nil {
		return "", err
	}
	return s, nil
}

// IsStale reports whether a stale_after value has been reached: stale means
// now >= stale_after, compared as instants (OKF section 5.5). An absent value is
// never stale, and neither is one that is not a valid OKF datetime — a consumer
// derives staleness only from the field as specified, and `okfw validate`
// is what reports the malformed value.
func IsStale(staleAfter string, now time.Time) bool {
	if staleAfter == "" {
		return false
	}
	cutoff, err := ParseDateTime(staleAfter)
	if err != nil {
		return false
	}
	return !now.Before(cutoff)
}

// frontmatterValue decodes a YAML node into plain Go values for schema
// validation, keeping every timestamp-looking scalar as the text the author
// wrote. Decoding straight into map[string]interface{} lets yaml.v3 resolve an
// unquoted 2026-12-31 to a time.Time, which then reaches CUE re-rendered as
// "2026-12-31T00:00:00Z" — so a bare date would pass a check written to reject
// it, while every reader of the file saw a date.
func frontmatterValue(n *yaml.Node) (interface{}, error) {
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return frontmatterValue(n.Content[0])
	case yaml.AliasNode:
		return frontmatterValue(n.Alias)
	case yaml.MappingNode:
		m := make(map[string]interface{}, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := frontmatterValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m[n.Content[i].Value] = v
		}
		return m, nil
	case yaml.SequenceNode:
		s := make([]interface{}, 0, len(n.Content))
		for _, c := range n.Content {
			v, err := frontmatterValue(c)
			if err != nil {
				return nil, err
			}
			s = append(s, v)
		}
		return s, nil
	case yaml.ScalarNode:
		if n.ShortTag() == "!!timestamp" {
			return n.Value, nil
		}
	}
	var v interface{}
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
