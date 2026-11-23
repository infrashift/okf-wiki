package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestCLIFlagReferenceIsCurrent: the docs site's CLI Flags page is generated
// from the command tree. A flag added, renamed or reworded without
// regenerating it fails here. OKFW_UPDATE_DOCS=1 (make docs-flags) rewrites it.
func TestCLIFlagReferenceIsCurrent(t *testing.T) {
	var buf bytes.Buffer
	genFlagReference(&buf, rootCmd())
	if os.Getenv("OKFW_UPDATE_DOCS") == "1" {
		if err := os.WriteFile(flagRefPath, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(flagRefPath)
	if err != nil {
		t.Fatalf("%v; run `make docs-flags`", err)
	}
	if !bytes.Equal(got, buf.Bytes()) {
		t.Errorf("%s is stale; run `make docs-flags`", flagRefPath)
	}
}

// TestFlagReferenceCoversEveryCommand: each available, runnable command gets a
// section, and each deprecated one is listed as deprecated rather than
// silently dropped.
func TestFlagReferenceCoversEveryCommand(t *testing.T) {
	var buf bytes.Buffer
	genFlagReference(&buf, rootCmd())
	page := buf.String()
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			switch {
			case sub.Deprecated != "":
				if !strings.Contains(page, "- `"+sub.CommandPath()+"`: ") {
					t.Errorf("deprecated %q is not listed as deprecated", sub.CommandPath())
				}
			case sub.IsAvailableCommand():
				if sub.Runnable() && !strings.Contains(page, "\n## "+sub.CommandPath()+"\n") {
					t.Errorf("no section for %q", sub.CommandPath())
				}
				walk(sub)
			}
		}
	}
	walk(rootCmd())
	if !strings.Contains(page, "`--project`, `-p`") || !strings.Contains(page, "**Deprecated:** use `--path`") {
		t.Error("the deprecated --project flag is not marked deprecated")
	}
}

// TestEveryFlagHasHelpText holds flag help to one style, since it is what
// --help, the man pages and the CLI Flags page all print: present, starting
// with a capital, and with no trailing period (cobra appends "(default …)").
func TestEveryFlagHasHelpText(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "help" {
				return
			}
			_, usage := pflag.UnquoteUsage(f)
			where := c.CommandPath() + " --" + f.Name
			switch {
			case strings.TrimSpace(usage) == "":
				t.Errorf("%s has no help text", where)
			case !unicode.IsUpper([]rune(usage)[0]):
				t.Errorf("%s: help text should start with a capital: %q", where, usage)
			case strings.HasSuffix(usage, "."):
				t.Errorf("%s: help text should not end with a period: %q", where, usage)
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd())
}
