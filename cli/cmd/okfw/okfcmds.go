package main

// Writers for the OKF v0.2 metadata families: lifecycle (section 5.4-5.5),
// trust (section 5.2), and provenance (section 5.1).
//
// Every family gets a writer here on purpose. `resource` spent months modelled,
// schema'd and documented with no way to set it — a field nothing can write is
// indistinguishable from a field that does not exist.

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

// resolveRefArg turns a concept id or path argument into an absolute file path.
func resolveRefArg(ref string) (*config.Config, string, error) {
	cfg, err := config.Discover("")
	if err != nil {
		return nil, "", err
	}
	path, err := bundle.ResolveConceptPath(cfg, ref)
	if err != nil {
		return nil, "", err
	}
	return cfg, path, nil
}

// editConcept applies fn to a concept's frontmatter, writes it back, and re-syncs.
func editConcept(ref string, fn func(m *yaml.Node) error) (string, error) {
	cfg, path, err := resolveRefArg(ref)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	out, err := bundle.Edit(content, func(m *yaml.Node) error {
		if err := fn(m); err != nil {
			return err
		}
		// Any of these edits is a meaningful change.
		stamp := time.Now().Format(time.RFC3339)
		bundle.MapSetString(m, "timestamp", stamp)
		if bundle.MapGet(m, "generated") != nil {
			if err := bundle.SetPath(m, "generated.at", stamp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if err := bundle.ValidateGenerated(out); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return "", err
	}
	if err := bundle.AutoSync(cfg, path); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to index: %v\n", err)
	}
	return path, nil
}

// defaultHumanActor returns "human:<login>" for the current user, the actor form
// OKF section 7 requires for hand-authored or human-confirmed content.
func defaultHumanActor() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return config.HumanActorPrefix + u.Username
	}
	return config.HumanActorPrefix + "unknown"
}

func newSetLifecycleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-lifecycle <id_or_file> <draft|stable|deprecated>",
		Short: "Set a concept's OKF lifecycle status",
		Long: `Set the OKF lifecycle status (spec section 5.4).

  draft       not yet reviewed; possibly incomplete
  stable      ready for consumption (the default when absent)
  deprecated  kept for links and history; no longer current

Examples:
  okfw set-lifecycle ./notes/old-api.md deprecated
  okfw set-lifecycle 019680f0 draft`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			status := args[1]
			switch status {
			case "draft", "stable", "deprecated":
			default:
				return fmt.Errorf("invalid status %q: want draft, stable, or deprecated", status)
			}
			path, err := editConcept(args[0], func(m *yaml.Node) error {
				bundle.MapSetString(m, "status", status)
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("Set status to %s: %s\n", status, path)
			return nil
		},
	}
}

func newSetStaleAfterCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "set-stale-after <id_or_file> [YYYY-MM-DD]",
		Short: "Set the date after which a concept counts as stale",
		Long: `Set stale_after (OKF spec section 5.5). A concept is stale when today is on
or after this date — an absolute date rather than a TTL, so staleness is a plain
comparison that needs no record of when the concept was read.

Examples:
  okfw set-stale-after ./notes/pricing.md 2026-12-31
  okfw set-stale-after ./notes/pricing.md --clear`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clear {
				path, err := editConcept(args[0], func(m *yaml.Node) error {
					bundle.MapDelete(m, "stale_after")
					return nil
				})
				if err != nil {
					return err
				}
				fmt.Printf("Cleared stale_after: %s\n", path)
				return nil
			}
			if len(args) < 2 {
				return fmt.Errorf("a date is required (or use --clear)")
			}
			date := args[1]
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return fmt.Errorf("invalid date %q: want YYYY-MM-DD", date)
			}
			path, err := editConcept(args[0], func(m *yaml.Node) error {
				bundle.MapSetString(m, "stale_after", date)
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("Set stale_after to %s: %s\n", date, path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "Remove stale_after instead of setting it")
	return cmd
}

func newVerifyCmd() *cobra.Command {
	var by, at string
	cmd := &cobra.Command{
		Use:   "verify <id_or_file>",
		Short: "Record a verification event on a concept",
		Long: `Append an entry to the concept's ` + "`verified`" + ` list (OKF spec section 5.2).

Consumers derive a trust tier from these entries: no verified key means
unverified, non-human actors mean machine-confirmed, and any human: actor means
human-reviewed. --by therefore defaults to human:<your login>, since running
this command by hand is a human confirmation.

Examples:
  okfw verify ./notes/pricing.md
  okfw verify 019680f0 --by human:ryan
  okfw verify 019680f0 --by process:nightly-audit`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actor := by
			if actor == "" {
				actor = defaultHumanActor()
			}
			stamp := at
			if stamp == "" {
				stamp = time.Now().Format(time.RFC3339)
			} else if _, err := time.Parse(time.RFC3339, stamp); err != nil {
				return fmt.Errorf("invalid --at %q: want an RFC 3339 datetime", stamp)
			}

			path, err := editConcept(args[0], func(m *yaml.Node) error {
				entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				bundle.MapSet(entry, "by", bundle.QuotedScalar(actor))
				bundle.MapSet(entry, "at", bundle.QuotedScalar(stamp))
				// A pre-existing bare mapping has to become a list before we can
				// append to it — OKF section 5.2 permits the single-entry
				// shorthand, so files in the wild carry both shapes.
				if cur := bundle.MapGet(m, "verified"); cur != nil && cur.Kind == yaml.MappingNode {
					bundle.MapSet(m, "verified", &yaml.Node{
						Kind: yaml.SequenceNode, Tag: "!!seq",
						Content: []*yaml.Node{cur, entry},
					})
					return nil
				}
				return bundle.AppendToSeq(m, "verified", entry)
			})
			if err != nil {
				return err
			}
			fmt.Printf("Verified by %s: %s\n", actor, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Actor: human:<id>, process:<id>, or <producer>/<version> (default human:<login>)")
	cmd.Flags().StringVar(&at, "at", "", "RFC 3339 datetime (default now)")
	return cmd
}

func newAddSourceCmd() *cobra.Command {
	var resource, id, title, author, lastModified, usageWindow string
	var usageCount int
	cmd := &cobra.Command{
		Use:   "add-source <id_or_file>",
		Short: "Append a provenance source to a concept",
		Long: `Append an entry to the concept's ` + "`sources`" + ` list (OKF spec section 5.1).

--resource is required and names either a concrete artifact a consumer can
follow (a URL or bundle-relative path) or a scope descriptor it cannot, such as
"all queries in project X".

--id is the join key for body footnotes: a markdown footnote labelled [^<id>]
attributes a specific claim to this source. Keyed rather than positional,
because reordering the list would silently misattribute every claim.

Examples:
  okfw add-source ./notes/ga4.md --resource https://example.com/schema --id ga4-schema
  okfw add-source 019680f0 --resource "all queries in project X" --usage-count 412 \
      --usage-window 2026-01-01..2026-06-30`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(resource) == "" {
				return fmt.Errorf("--resource is required within a source entry")
			}
			var from, to string
			if usageWindow != "" {
				parts := strings.SplitN(usageWindow, "..", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid --usage-window %q: want FROM..TO", usageWindow)
				}
				from, to = parts[0], parts[1]
				for _, d := range []string{from, to} {
					if _, err := time.Parse("2006-01-02", d); err != nil {
						return fmt.Errorf("invalid --usage-window date %q: want YYYY-MM-DD", d)
					}
				}
			}
			if lastModified != "" {
				if _, err := time.Parse("2006-01-02", lastModified); err != nil {
					return fmt.Errorf("invalid --last-modified %q: want YYYY-MM-DD", lastModified)
				}
			}

			path, err := editConcept(args[0], func(m *yaml.Node) error {
				entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				bundle.MapSet(entry, "resource", bundle.QuotedScalar(resource))
				for _, kv := range []struct{ k, v string }{
					{"id", id}, {"title", title}, {"author", author},
					{"last_modified", lastModified},
				} {
					if kv.v != "" {
						bundle.MapSet(entry, kv.k, bundle.QuotedScalar(kv.v))
					}
				}
				if cmd.Flags().Changed("usage-count") {
					bundle.MapSet(entry, "usage_count", &yaml.Node{
						Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(usageCount),
					})
				}
				if from != "" {
					win := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
					bundle.MapSet(win, "from", bundle.QuotedScalar(from))
					bundle.MapSet(win, "to", bundle.QuotedScalar(to))
					bundle.MapSet(entry, "usage_window", win)
				}
				return bundle.AppendToSeq(m, "sources", entry)
			})
			if err != nil {
				return err
			}
			fmt.Printf("Added source %s: %s\n", resource, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&resource, "resource", "", "URL, bundle-relative path, or scope descriptor (required)")
	cmd.Flags().StringVar(&id, "id", "", "Join key for [^id] body footnotes")
	cmd.Flags().StringVar(&title, "title", "", "Human-readable label")
	cmd.Flags().StringVar(&author, "author", "", "Actor who produced the source")
	cmd.Flags().IntVar(&usageCount, "usage-count", 0, "How often the resource was exercised")
	cmd.Flags().StringVar(&lastModified, "last-modified", "", "When the source itself last changed (YYYY-MM-DD)")
	cmd.Flags().StringVar(&usageWindow, "usage-window", "", "Date range framing usage-count, as FROM..TO")
	return cmd
}

func newSetResourceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-resource <id_or_file> <uri>",
		Short: "Set the URI of the asset a concept describes",
		Long: `Set the OKF ` + "`resource`" + ` field: a URI uniquely identifying the underlying
asset this concept describes. Absent for concepts about abstract ideas rather
than physical resources.

Examples:
  okfw set-resource ./notes/orders-table.md bigquery://project.dataset.orders
  okfw set-resource 019680f0 https://example.com/api/v2`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := editConcept(args[0], func(m *yaml.Node) error {
				bundle.MapSetString(m, "resource", args[1])
				return nil
			})
			if err != nil {
				return err
			}
			fmt.Printf("Set resource: %s\n", path)
			return nil
		},
	}
}

func newSetGeneratedCmd() *cobra.Command {
	var by string
	cmd := &cobra.Command{
		Use:   "set-generated <id_or_file>",
		Short: "Record who generated a concept",
		Long: `Set ` + "`generated.by`" + ` (OKF spec section 5.2), the actor that produced the
concept's content. generated.at is kept equal to the concept's timestamp — both
record when the content last meaningfully changed — so it is not set directly.

--by defaults to human:<your login>. okfw does not stamp this on create: a note
you wrote by hand was authored by you, not by the tool, and inventing an
attribution is worse than leaving it absent.

Examples:
  okfw set-generated ./notes/pricing.md
  okfw set-generated 019680f0 --by process:finance-nightly`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			actor := by
			if actor == "" {
				actor = defaultHumanActor()
			}
			path, err := editConcept(args[0], func(m *yaml.Node) error {
				return bundle.SetPath(m, "generated.by", actor)
			})
			if err != nil {
				return err
			}
			fmt.Printf("Set generated.by to %s: %s\n", actor, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Actor: human:<id>, process:<id>, or <producer>/<version> (default human:<login>)")
	return cmd
}

// okfFamilyCommands returns the writers for the OKF metadata families.
func okfFamilyCommands() []*cobra.Command {
	return []*cobra.Command{
		newSetLifecycleCmd(),
		newSetStaleAfterCmd(),
		newVerifyCmd(),
		newAddSourceCmd(),
		newSetResourceCmd(),
		newSetGeneratedCmd(),
	}
}
