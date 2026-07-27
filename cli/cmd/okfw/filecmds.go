package main

// `okfw delete` and `okfw move`: the two operations that change whether a concept
// exists and where. Both keep the file, the search index, the graph database, and
// the index.md listings in step; doing any one of them by hand leaves the other
// three stale.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

func newDeleteCmd() *cobra.Command {
	var force, asJSON bool
	cmd := &cobra.Command{
		Use:     "delete <id_or_file>",
		Aliases: []string{"rm"},
		Short:   "Delete a concept and everything derived from it",
		Long: `Remove a concept's file, its search-index entry, its graph node, and its
line in the index.md listings.

Concepts that link to the deleted one are reported but left alone. OKF requires
consumers to tolerate a link whose target does not exist, so a dangling link is
not malformed — and rewriting someone's prose to tidy one would destroy authored
content.

This destroys a file, so it asks for confirmation unless --force is given, and
refuses to prompt when stdin is not a terminal.

Examples:
  okfw delete ./notes/scratch--019f0494.md
  okfw delete 019f0494 --force
  okfw delete 019f0494 --force --json    # {"id","path","inbound":[...]}`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, absPath, err := resolveRefArg(args[0])
			if err != nil {
				return err
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(absPath, cfg.RootPath), string(os.PathSeparator))

			if !force {
				ok, cerr := confirm(cmd, fmt.Sprintf("Delete %s?", rel))
				if cerr != nil {
					return cerr
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
					return nil
				}
			}

			res, err := bundle.DeleteConcept(cfg, absPath)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, map[string]interface{}{
					"id":      res.ID,
					"path":    absPath,
					"inbound": orEmpty(res.Inbound),
				})
			}
			fmt.Fprintf(out, "Deleted %s\n", res.Rel)
			if n := len(res.Inbound); n > 0 {
				fmt.Fprintf(out, "\n%d concept(s) still link to it (OKF tolerates broken links):\n", n)
				for _, src := range res.Inbound {
					fmt.Fprintf(out, "  %s\n", src)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Delete without confirming")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the result as JSON")
	return cmd
}

func newMoveCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:     "move <id_or_file> <destination>",
		Aliases: []string{"rename", "mv"},
		Short:   "Move or rename a concept, repairing links at both ends",
		Long: `Relocate a concept within the bundle. Its ` + "`id`" + ` does not change, so
hierarchies built with ` + "`parent`" + ` survive untouched.

The destination works like mv: a path ending in .md is the concept's new full
path, anything else is a directory to move it into under its current filename.
Both are bundle-relative and may not escape the bundle or name one of its
internal dot-directories.

Links are repaired at both ends, since a move breaks two different sets:

  - Other concepts pointing at the old path are retargeted, keeping whichever
    form they used (bundle-absolute or relative).
  - The moved concept's own *relative* links are rewritten so they still resolve
    from the new directory. Bundle-absolute links need no fixing, which is why
    OKF recommends them.

The moved concept's timestamp is bumped, because a concept's OKF identity is its
path within the bundle. Concepts whose links were merely retargeted are not
stamped: their knowledge did not change.

Examples:
  okfw move 019f0494 archive/                        # into a directory
  okfw move ./scratch--019f0494.md notes/design.md   # new path and name
  okfw rename 019f0494 notes/better-name.md
  okfw move 019f0494 archive/ --json                 # {"from","to","rewrote":[...]}`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, oldPath, err := resolveRefArg(args[0])
			if err != nil {
				return err
			}
			newPath, err := resolveDestination(cfg, oldPath, args[1])
			if err != nil {
				return err
			}

			res, err := bundle.MoveConcept(cfg, oldPath, newPath)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if asJSON {
				return writeJSON(out, map[string]interface{}{
					"from":      oldPath,
					"to":        newPath,
					"from_rel":  res.FromRel,
					"to_rel":    res.ToRel,
					"rewrote":   orEmpty(res.Rewrote),
					"self_link": res.SelfLink,
				})
			}
			fmt.Fprintf(out, "Moved %s -> %s\n", res.FromRel, res.ToRel)
			if res.SelfLink {
				fmt.Fprintln(out, "  (its self-link was retargeted)")
			}
			if n := len(res.Rewrote); n > 0 {
				fmt.Fprintf(out, "Rewrote links in %d concept(s):\n", n)
				for _, rel := range res.Rewrote {
					fmt.Fprintf(out, "  %s\n", rel)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the result as JSON")
	return cmd
}

// writeJSON emits a compact JSON object followed by a newline.
func writeJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "%s\n", data)
	return err
}

// orEmpty keeps a nil slice out of the JSON as `null`, so consumers can always
// iterate the field.
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// resolveDestination turns a move destination argument into an absolute path,
// following mv's convention: a .md target names the file, anything else names a
// directory to move into.
func resolveDestination(cfg *config.Config, oldPath, dest string) (string, error) {
	trimmed := strings.TrimSpace(filepath.ToSlash(dest))
	if trimmed == "" {
		return "", fmt.Errorf("a destination is required")
	}

	// An existing directory, or a trailing slash, or no .md suffix: move into it.
	intoDir := strings.HasSuffix(trimmed, "/") || !strings.HasSuffix(trimmed, ".md")
	if !intoDir {
		if fi, err := os.Stat(filepath.Join(cfg.RootPath, filepath.FromSlash(trimmed))); err == nil && fi.IsDir() {
			intoDir = true
		}
	}

	if intoDir {
		sub := bundle.SanitizeSubdir(trimmed)
		if sub == "" {
			return "", fmt.Errorf("invalid destination %q: must be a subdirectory within the bundle", dest)
		}
		return filepath.Join(cfg.RootPath, filepath.FromSlash(sub), filepath.Base(oldPath)), nil
	}

	dir, base := path.Split(trimmed)
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		// Renaming in place at the bundle root.
		return filepath.Join(cfg.RootPath, base), nil
	}
	sub := bundle.SanitizeSubdir(dir)
	if sub == "" {
		return "", fmt.Errorf("invalid destination %q: must be a subdirectory within the bundle", dest)
	}
	return filepath.Join(cfg.RootPath, filepath.FromSlash(sub), base), nil
}

// confirm asks a yes/no question, defaulting to no.
//
// Two layers keep an unattended run from destroying anything: piped or
// redirected-from-file input is refused outright, and anything that is not an
// explicit yes — including EOF from an empty or closed stdin — answers no. The
// char-device test does not distinguish a terminal from /dev/null, so the
// no-by-default rule is what actually carries the guarantee.
func confirm(cmd *cobra.Command, prompt string) (bool, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok {
		fi, serr := f.Stat()
		if serr != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false, fmt.Errorf("stdin is not a terminal; pass --force to delete without confirming")
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", prompt)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	}
	return false, nil
}

// fileCommands returns the commands that change whether a concept exists and where.
func fileCommands() []*cobra.Command {
	return []*cobra.Command{newDeleteCmd(), newMoveCmd()}
}
