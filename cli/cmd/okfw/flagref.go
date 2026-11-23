package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagRefPath is the docs page genFlagReference writes, relative to this
// package. TestCLIFlagReferenceIsCurrent fails when it is stale, and
// `make docs-flags` rewrites it.
const flagRefPath = "../../../docs/src/content/docs/reference/cli-flags.md"

// genFlagReference renders every command's flags as the docs site's CLI
// Flags page, from the same command tree as --help and the man pages. The
// hand-written tables it replaced had drifted: commands with no entry, flags
// listed on the wrong subcommand, and a deprecated flag still documented.
func genFlagReference(w io.Writer, root *cobra.Command) {
	p := func(format string, a ...interface{}) { fmt.Fprintf(w, format, a...) }
	p("---\ntitle: CLI Flags\ndescription: Every okfw command and its flags, generated from the command tree.\n---\n\n")
	p("<!-- Generated from okfw's command tree by `make docs-flags`. Do not edit: change the flag's help text instead. -->\n\n")
	p("This page lists each okfw command and its flags. okfw makes it from its command tree, the same source as `okfw <command> --help` and the man pages. ")
	p("So the page cannot list a flag that okfw does not have. ")
	p("For the purpose of each command, with examples, see [CLI Commands](/okf-wiki/reference/cli-commands/).\n\n")
	p("Each command also takes `--help` (`-h`). Most commands need a wiki. They look for `.okfw/` in the current directory and then in each parent directory. When they find none, they stop with an error that tells you to run `okfw init`.\n\n")

	p("## Global flags\n\n")
	writeFlagTable(w, root.LocalFlags(), root)

	var deprecated []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.Deprecated != "" {
				deprecated = append(deprecated, sub)
				continue
			}
			if !sub.IsAvailableCommand() {
				continue
			}
			if sub.Runnable() {
				writeCommand(w, sub, root)
			}
			walk(sub)
		}
	}
	walk(root)

	if len(deprecated) > 0 {
		p("## Deprecated commands\n\n")
		p("These commands still run, and print a warning. `--help` and the man pages do not show them.\n\n")
		for _, c := range deprecated {
			p("- `%s`: %s\n", c.CommandPath(), mdText(c.Deprecated))
		}
	}
}

func writeCommand(w io.Writer, c, root *cobra.Command) {
	fmt.Fprintf(w, "## %s\n\n", c.CommandPath())
	fmt.Fprintf(w, "%s.\n\n", mdText(strings.TrimSuffix(c.Short, ".")))
	fmt.Fprintf(w, "```\n%s\n```\n\n", strings.TrimSuffix(c.UseLine(), " [flags]"))
	if len(c.Aliases) > 0 {
		fmt.Fprintf(w, "Aliases: `%s`.\n\n", strings.Join(c.Aliases, "`, `"))
	}
	flags := pflag.NewFlagSet(c.Name(), pflag.ContinueOnError)
	c.NonInheritedFlags().VisitAll(func(f *pflag.Flag) { flags.AddFlag(f) })
	// Inherited from a parent command, not from the root: okfw site's --bun
	// applies to site build, up, down and status alike.
	rootFlags := root.PersistentFlags()
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if rootFlags.Lookup(f.Name) == nil {
			flags.AddFlag(f)
		}
	})
	writeFlagTable(w, flags, c)
}

// writeFlagTable writes one row per flag, deprecated flags last. --help is
// left out: every command has it, and it filled a row in every table.
func writeFlagTable(w io.Writer, fs *pflag.FlagSet, c *cobra.Command) {
	var live, old []*pflag.Flag
	fs.VisitAll(func(f *pflag.Flag) {
		switch {
		case f.Name == "help":
		case f.Deprecated != "":
			old = append(old, f)
		case !f.Hidden:
			live = append(live, f)
		}
	})
	sort.Slice(live, func(i, j int) bool { return live[i].Name < live[j].Name })
	if len(live)+len(old) == 0 {
		fmt.Fprint(w, "No flags.\n\n")
		return
	}
	fmt.Fprint(w, "| Flag | Value | Default | Description |\n|---|---|---|---|\n")
	row := func(f *pflag.Flag, desc string) {
		name := "`--" + f.Name + "`"
		if f.Shorthand != "" {
			name += ", `-" + f.Shorthand + "`"
		}
		value, _ := pflag.UnquoteUsage(f)
		if value != "" {
			value = "`" + value + "`"
		}
		def := ""
		switch f.DefValue {
		case "", "false", "[]", "0":
		default:
			def = "`" + strings.ReplaceAll(f.DefValue, "|", `\|`) + "`"
		}
		if from := owner(f, c); from != "" {
			desc += ". Applies to every `" + from + "` subcommand"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s |\n", name, value, def, desc)
	}
	for _, f := range live {
		_, usage := pflag.UnquoteUsage(f)
		row(f, mdText(usage))
	}
	for _, f := range old {
		_, usage := pflag.UnquoteUsage(f)
		row(f, "**Deprecated:** "+mdText(f.Deprecated)+". Was: "+mdText(usage))
	}
	fmt.Fprintln(w)
}

// owner names the ancestor of c that declares f as a persistent flag, or ""
// when c declares it itself.
func owner(f *pflag.Flag, c *cobra.Command) string {
	if c.NonInheritedFlags().Lookup(f.Name) == f {
		return ""
	}
	for p := c.Parent(); p != nil; p = p.Parent() {
		if p.PersistentFlags().Lookup(f.Name) == f {
			return p.CommandPath()
		}
	}
	return ""
}

// codeToken is what mdText sets in code: a flag name, and a token holding a
// <placeholder>. Astro's typography turns a bare -- into an en dash and reads
// <dir> as an HTML tag.
var codeToken = regexp.MustCompile(`--[a-z][a-z0-9-]*|[^\s,;()]*<[^>\s]+>[^\s,;()]*`)

// mdText makes help text safe and legible in a markdown table cell.
func mdText(s string) string {
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "|", `\|`)
	var b strings.Builder
	last := 0
	for _, m := range codeToken.FindAllStringIndex(s, -1) {
		b.WriteString(esc.Replace(s[last:m[0]]))
		b.WriteString("`" + strings.ReplaceAll(s[m[0]:m[1]], "|", `\|`) + "`")
		last = m[1]
	}
	b.WriteString(esc.Replace(s[last:]))
	return b.String()
}
