package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestEveryCommandHasAManPage: the pages are generated from the command tree,
// so a command without one would mean the generator skipped it.
func TestEveryCommandHasAManPage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SOURCE_DATE_EPOCH", "1767225600")
	if _, err := genManPages(dir); err != nil {
		t.Fatal(err)
	}
	var walk func(c *cobra.Command, prefix string)
	walk = func(c *cobra.Command, prefix string) {
		if !c.IsAvailableCommand() && c.Parent() != nil {
			return
		}
		name := prefix + c.Name()
		page := filepath.Join(dir, name+".1")
		data, err := os.ReadFile(page)
		if err != nil {
			t.Errorf("no man page for %q", strings.ReplaceAll(name, "-", " "))
			return
		}
		if !strings.Contains(string(data), ".TH \"OKFW\" \"1\" \"Jan 2026\"") {
			t.Errorf("%s: header not dated from SOURCE_DATE_EPOCH:\n%.200s", page, data)
		}
		for _, sub := range c.Commands() {
			walk(sub, name+"-")
		}
	}
	walk(rootCmd(), "")
}
