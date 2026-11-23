package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/infrashift/okf-wiki/internal/config"
)

// genManPages writes okfw.1 and one page per subcommand into dir, from the
// command tree itself, so a page cannot describe a flag the binary lacks.
// SOURCE_DATE_EPOCH, when set, dates the pages, so a release build is
// reproducible.
func genManPages(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	date := time.Now()
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			date = time.Unix(n, 0).UTC()
		}
	}
	root := rootCmd()
	root.DisableAutoGenTag = true
	header := &doc.GenManHeader{
		Title:   "OKFW",
		Section: "1",
		Source:  "okfw " + config.Version(),
		Manual:  "OKF Wiki Manual",
		Date:    &date,
	}
	if err := doc.GenManTree(root, header, dir); err != nil {
		return 0, err
	}
	pages, _ := filepath.Glob(filepath.Join(dir, "okfw*.1"))
	return len(pages), nil
}

// defaultManDir is where `okfw man` installs pages for the current user:
// $XDG_DATA_HOME/man/man1, else ~/.local/share/man/man1. man(1) searches the
// share/man beside each bin directory on PATH, so with okfw in ~/.local/bin,
// `man okfw` works with no MANPATH change.
func defaultManDir() (string, error) {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "man", "man1"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "man", "man1"), nil
}

func newManCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "man",
		Short: "Install okfw's man pages",
		Long: `Write okfw's manual pages — okfw(1) and one page per subcommand, such as
okfw-search(1) — generated from the commands themselves.

By default they go to ~/.local/share/man/man1 (or $XDG_DATA_HOME/man/man1).
man searches the share/man directory beside each bin directory on PATH, so
with okfw installed in ~/.local/bin, "man okfw" then works without setting
MANPATH. Release archives and "make install" ship the same pages under
share/man/man1.

Examples:
  okfw man                      # install for the current user
  okfw man --dir ./man/man1     # write them somewhere else`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				d, err := defaultManDir()
				if err != nil {
					return err
				}
				dir = d
			}
			n, err := genManPages(dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d man page(s) to %s\n", n, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory to write the pages to (default ~/.local/share/man/man1)")
	return cmd
}
