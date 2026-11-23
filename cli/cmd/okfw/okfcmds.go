package main

// Writers for the OKF v0.3 metadata families: lifecycle (section 5.4-5.5),
// trust (section 5.2), and provenance (section 5.1).
//
// Every family gets a writer here on purpose. `resource` spent months modelled,
// schema'd and documented with no way to set it — a field nothing can write is
// indistinguishable from a field that does not exist.

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/infrashift/okf-wiki/cli/internal/bundle"
	"github.com/infrashift/okf-wiki/cli/internal/okfwskills"
	"github.com/infrashift/okf-wiki/internal/config"
	"github.com/infrashift/okf-wiki/internal/graph"
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
		// okfw's timestamp records every write. generated.at is left alone:
		// these edit metadata, not the content a reviewer read (OKF v0.3
		// section 5.2), so a verification stays current across them.
		bundle.MapSetString(m, "timestamp", time.Now().Format(time.RFC3339))
		return nil
	})
	if err != nil {
		return "", err
	}
	// Guarded, not unconditional: an edit must not break a valid concept, but
	// these writers must still be able to repair an already-broken one. Holding
	// them to ValidateGenerated meant `okfw set-lifecycle` on a malformed file
	// refused, so the CLI had no way to fix it.
	if err := bundle.GuardMutation(content, out); err != nil {
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
	var supersededBy string
	cmd := &cobra.Command{
		Use:   "set-lifecycle <id_or_file> <draft|stable|deprecated>",
		Short: "Set a concept's OKF lifecycle status",
		Long: `Set the OKF lifecycle status (spec section 5.4).

  draft       not yet reviewed; possibly incomplete
  stable      ready for consumption (the default when absent)
  deprecated  kept for links and history; no longer current

--superseded-by names the concept that replaces a deprecated one (any
reference okfw accepts: an id, an OKF concept ID or a path). It is written as
a bundle-absolute path, and consumers filtering to current knowledge follow
it to the replacement (okfw search --current does). It has a meaning only for
a deprecated concept, so setting any other status removes it.

Examples:
  okfw set-lifecycle ./notes/old-api.md deprecated --superseded-by ./notes/api-v2.md
  okfw set-lifecycle 019680f0 draft`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}
			mut, err := lifecycleMutator(cfg, args[1], supersededBy)
			if err != nil {
				return err
			}
			path, err := editConcept(args[0], mut)
			if err != nil {
				return err
			}
			fmt.Printf("Set status to %s: %s\n", args[1], path)
			return nil
		},
	}
	cmd.Flags().StringVar(&supersededBy, "superseded-by", "", "With deprecated: the concept that replaces this one (id or path)")
	return cmd
}

// lifecycleMutator sets status and, for a deprecated concept, superseded_by:
// the replacing concept's bundle-absolute path (OKF v0.3 section 5.4), which
// stays valid if the deprecated concept itself is later moved. The replacement
// must exist. Any other status drops superseded_by, which means nothing
// without deprecated.
func lifecycleMutator(cfg *config.Config, status, supersededBy string) (func(*yaml.Node) error, error) {
	switch status {
	case "draft", "stable", "deprecated":
	default:
		return nil, fmt.Errorf("invalid status %q: want draft, stable, or deprecated", status)
	}
	target := ""
	if supersededBy != "" {
		if status != "deprecated" {
			return nil, fmt.Errorf("superseded_by needs status deprecated (OKF section 5.4)")
		}
		p, err := bundle.ResolveConceptPath(cfg, supersededBy)
		if err != nil {
			return nil, fmt.Errorf("superseded_by: %w", err)
		}
		rel, err := filepath.Rel(cfg.RootPath, p)
		if err != nil {
			return nil, err
		}
		target = "/" + filepath.ToSlash(rel)
	}
	return func(m *yaml.Node) error {
		bundle.MapSetString(m, "status", status)
		switch {
		case target != "":
			bundle.MapSetString(m, "superseded_by", target)
		case status != "deprecated":
			bundle.MapDelete(m, "superseded_by")
		}
		return nil
	}, nil
}

func newSetBundleMetaCmd() *cobra.Command {
	var title, description, license string
	cmd := &cobra.Command{
		Use:   "set-bundle-meta",
		Short: "Set the bundle's title, description or license",
		Long: `Set the bundle metadata OKF v0.3 section 8.1 keeps in the frontmatter of the
bundle-root index.md. Every other key there is kept as it is.

  --title        a display name for the bundle
  --description  one sentence on what the bundle covers and when to use it;
                 an agent with access to several bundles selects one by it
  --license      the license of the bundle content: an SPDX expression or a
                 path to a license file in the bundle

okfw validate requires a title and a description. Pass an empty value to
remove a key. okf_version is not set here: okfw migrate moves it, since it
decides how bare path fields read.

Examples:
  okfw set-bundle-meta --description "Runbooks and design decisions for billing."
  okfw set-bundle-meta --title Billing --license CC-BY-4.0`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}
			set := map[string]string{}
			for flag, key := range map[string]string{"title": "title", "description": "description", "license": "license"} {
				if cmd.Flags().Changed(flag) {
					v, _ := cmd.Flags().GetString(flag)
					set[key] = v
				}
			}
			if len(set) == 0 {
				return fmt.Errorf("nothing to set: pass --title, --description or --license")
			}
			if err := bundle.SetBundleMeta(cfg, set); err != nil {
				return err
			}
			fmt.Printf("Updated %s\n", filepath.Join(cfg.RootPath, "index.md"))
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Bundle title")
	cmd.Flags().StringVar(&description, "description", "", "One sentence: what the bundle covers and when to use it")
	cmd.Flags().StringVar(&license, "license", "", "SPDX expression, or a path to a license file in the bundle")
	return cmd
}

func newSetStaleAfterCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "set-stale-after <id_or_file> [DATETIME]",
		Short: "Set the instant after which a concept counts as stale",
		Long: `Set stale_after (OKF spec section 5.5). A concept is stale once now is on or
after this instant — an absolute instant rather than a TTL, so staleness is a
plain comparison that needs no record of when the concept was read.

OKF requires an ISO 8601 datetime with an explicit UTC offset. A bare
YYYY-MM-DD is accepted here and written as midnight UTC.

Examples:
  okfw set-stale-after ./notes/pricing.md 2026-12-31T00:00:00Z
  okfw set-stale-after ./notes/pricing.md 2026-12-31          # 2026-12-31T00:00:00Z
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
				return fmt.Errorf("a datetime is required (or use --clear)")
			}
			date, err := config.NormalizeDateTime(args[1])
			if err != nil {
				return fmt.Errorf("invalid stale_after: %w", err)
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

			path, err := editConcept(args[0], verifyMutator(actor, stamp))
			if err != nil {
				return err
			}
			fmt.Printf("Verified by %s: %s\n", actor, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Actor: human:<id>, team:<id>, process:<id>, or <producer>/<version> (default human:<login>)")
	cmd.Flags().StringVar(&at, "at", "", "When it was verified: an RFC 3339 datetime or a date (default now)")
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
      --usage-window 2026-01-01T00:00:00Z..2026-06-30T00:00:00Z

Datetimes are ISO 8601 with a UTC offset, as OKF requires; a bare YYYY-MM-DD
is accepted and written as midnight UTC.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(resource) == "" {
				return fmt.Errorf("--resource is required within a source entry")
			}
			from, to, err := parseUsageWindow(usageWindow)
			if err != nil {
				return err
			}
			if lastModified, err = normalizeOptionalDateTime("last_modified", lastModified); err != nil {
				return err
			}

			path, err := editConcept(args[0], addSourceMutator(sourceEntry{
				resource: resource, id: id, title: title, author: author,
				lastModified: lastModified,
				usageCount:   usageCount, hasUsage: cmd.Flags().Changed("usage-count"),
				from: from, to: to,
			}))
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
	cmd.Flags().StringVar(&lastModified, "last-modified", "", "When the source itself last changed (ISO 8601 datetime)")
	cmd.Flags().StringVar(&usageWindow, "usage-window", "", "Datetime range framing usage-count, as FROM..TO")
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
			path, err := editConcept(args[0], setResourceMutator(args[1]))
			if err != nil {
				return err
			}
			fmt.Printf("Set resource: %s\n", path)
			return nil
		},
	}
}

func newSetGeneratedCmd() *cobra.Command {
	var by, at string
	cmd := &cobra.Command{
		Use:   "set-generated <id_or_file>",
		Short: "Record who generated a concept",
		Long: `Record a generation (OKF spec section 5.2): generated.by, the actor that
produced the concept's content, and generated.at, when it last meaningfully
changed (now, unless --at says otherwise).

generated.at is what decides whether a verification still counts: OKF v0.3
section 5.3 counts only verified events at or after it. Recording a new
generation therefore makes earlier reviews history, which is the point — the
reviewer did not read the new text. okfw moves generated.at itself only when a
concept's body is rewritten through okfw (the MCP update tool's append and
set_body); tag, lifecycle, source and verify edits, and the link repair okfw
move does, leave it alone. okfw's own timestamp still records every write.

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
			stamp, err := normalizeOptionalDateTime("--at", at)
			if err != nil {
				return err
			}
			path, err := editConcept(args[0], setGeneratedMutator(actor, stamp))
			if err != nil {
				return err
			}
			fmt.Printf("Set generated.by to %s: %s\n", actor, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "", "Actor: human:<id>, team:<id>, process:<id>, or <producer>/<version> (default human:<login>)")
	cmd.Flags().StringVar(&at, "at", "", "When the content last meaningfully changed: an RFC 3339 datetime or a date (default now)")
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

// The OKF-family mutators, extracted so the MCP `update` tool applies the same
// frontmatter edits as the CLI writers.
//
// CLAUDE.md states that `update` stays at parity with the CLI writers, and it
// did not: verify, add-source, set-resource and set-generated had no MCP
// action, so two of the three OKF v0.2 metadata families — trust and
// provenance — were unreachable to an agent. Two implementations of the same
// edit is how the two would drift again, so there is one.

// verifyMutator appends a verification entry (OKF section 5.2).
func verifyMutator(actor, stamp string) func(*yaml.Node) error {
	return func(m *yaml.Node) error {
		// Checked here, not left to the write guard, which lets any edit
		// through to an already-invalid concept.
		if err := config.ValidateActor(actor); err != nil {
			return err
		}
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		bundle.MapSet(entry, "by", bundle.QuotedScalar(actor))
		bundle.MapSet(entry, "at", bundle.QuotedScalar(stamp))
		// A pre-existing bare mapping has to become a list before we can append
		// to it — OKF section 5.2 permits the single-entry shorthand, so files
		// in the wild carry both shapes.
		if cur := bundle.MapGet(m, "verified"); cur != nil && cur.Kind == yaml.MappingNode {
			bundle.MapSet(m, "verified", &yaml.Node{
				Kind: yaml.SequenceNode, Tag: "!!seq",
				Content: []*yaml.Node{cur, entry},
			})
			return nil
		}
		return bundle.AppendToSeq(m, "verified", entry)
	}
}

// sourceEntry is one provenance source (OKF section 5.1).
type sourceEntry struct {
	resource     string
	id           string
	title        string
	author       string
	lastModified string
	usageCount   int
	hasUsage     bool
	from, to     string
}

// addSourceMutator appends a provenance source.
func addSourceMutator(e sourceEntry) func(*yaml.Node) error {
	return func(m *yaml.Node) error {
		// Refused here as well as by validation: the write guard lets edits
		// through to an already-invalid concept, and a second entry with this
		// id would make every [^id] citation ambiguous.
		if e.id != "" {
			if seq := bundle.MapGet(m, "sources"); seq != nil && seq.Kind == yaml.SequenceNode {
				for _, have := range seq.Content {
					if have.Kind != yaml.MappingNode {
						continue
					}
					if id := bundle.MapGet(have, "id"); id != nil && graph.FootnoteKey(id.Value) == graph.FootnoteKey(e.id) {
						return fmt.Errorf("sources already has an entry with id %q; a [^%s] footnote could not tell them apart",
							id.Value, e.id)
					}
				}
			}
		}
		entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		bundle.MapSet(entry, "resource", bundle.QuotedScalar(e.resource))
		for _, kv := range []struct{ k, v string }{
			{"id", e.id}, {"title", e.title}, {"author", e.author},
			{"last_modified", e.lastModified},
		} {
			if kv.v != "" {
				bundle.MapSet(entry, kv.k, bundle.QuotedScalar(kv.v))
			}
		}
		if e.hasUsage {
			bundle.MapSet(entry, "usage_count", &yaml.Node{
				Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(e.usageCount),
			})
		}
		if e.from != "" {
			win := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			bundle.MapSet(win, "from", bundle.QuotedScalar(e.from))
			bundle.MapSet(win, "to", bundle.QuotedScalar(e.to))
			bundle.MapSet(entry, "usage_window", win)
		}
		return bundle.AppendToSeq(m, "sources", entry)
	}
}

// setResourceMutator sets the URI of the asset a concept describes.
func setResourceMutator(uri string) func(*yaml.Node) error {
	return func(m *yaml.Node) error {
		bundle.MapSetString(m, "resource", uri)
		return nil
	}
}

// setGeneratedMutator records a generation: generated.by, and generated.at at
// the given instant (now when empty). Recording a generation is the explicit
// statement that the content was (re)produced, so it moves generated.at, and a
// verification made before it stops counting (OKF v0.3 section 5.3).
func setGeneratedMutator(actor, at string) func(*yaml.Node) error {
	return func(m *yaml.Node) error {
		if err := config.ValidateActor(actor); err != nil {
			return err
		}
		if at == "" {
			at = time.Now().Format(time.RFC3339)
		}
		if err := bundle.SetPath(m, "generated.by", actor); err != nil {
			return err
		}
		return bundle.SetPath(m, "generated.at", at)
	}
}

// parseUsageWindow splits a FROM..TO window and normalizes both ends to OKF
// datetimes.
func parseUsageWindow(s string) (from, to string, err error) {
	if s == "" {
		return "", "", nil
	}
	parts := strings.SplitN(s, "..", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid usage window %q: want FROM..TO", s)
	}
	if from, err = config.NormalizeDateTime(parts[0]); err != nil {
		return "", "", fmt.Errorf("invalid usage window: %w", err)
	}
	if to, err = config.NormalizeDateTime(parts[1]); err != nil {
		return "", "", fmt.Errorf("invalid usage window: %w", err)
	}
	return from, to, nil
}

// normalizeOptionalDateTime normalizes a datetime that may be left empty.
func normalizeOptionalDateTime(field, s string) (string, error) {
	if s == "" {
		return "", nil
	}
	v, err := config.NormalizeDateTime(s)
	if err != nil {
		return "", fmt.Errorf("invalid %s: %w", field, err)
	}
	return v, nil
}

// newInstallCmd registers okfw with an agent platform.
//
// The skill used to be a loose file at the repo root that README told the
// reader to copy by hand, with no CLAUDE.md block and no drift test — so an
// agent given the okfw MCP server got five tool schemas and none of the
// conventions that make the wiki worth having.
func newInstallCmd() *cobra.Command {
	var platform string
	var force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the okfw skill for an AI coding assistant",
		Long: `Registers okfw with an agent platform so it captures and links project
knowledge in the wiki instead of scattering notes. Supported platforms: claude
(Claude Code: skill + CLAUDE.md block), gemini (GEMINI.md block), codex
(AGENTS.md block). Managed marker blocks keep re-installs idempotent, and the
block is namespaced, so it coexists with blocks other tools maintain in the same file.

Run from anywhere inside the project; the wiki's parent directory is the root.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Discover("")
			if err != nil {
				return err
			}
			// The bundle is <root>/.okfw, so the project root is its parent.
			root := filepath.Dir(cfg.RootPath)
			res, err := okfwskills.Install(okfwskills.Options{
				Root:     root,
				Platform: platform,
				Force:    force,
			})
			if err != nil {
				return err
			}
			for _, f := range res.Written {
				fmt.Println("Wrote", f)
			}
			for _, n := range res.Notes {
				fmt.Println("Note:", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&platform, "platform", "claude", "agent platform: claude, gemini, or codex")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing skill file")
	return cmd
}
