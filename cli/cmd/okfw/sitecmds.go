package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/infrashift/okf-wiki/cli/internal/site"
	"github.com/infrashift/okf-wiki/internal/config"
)

// siteCacheDir overrides where site workdirs live; tests point it at a temp
// directory.
var siteCacheDir = ""

func newSiteCmd() *cobra.Command {
	var bunPath string
	cmd := &cobra.Command{
		Use:   "site",
		Short: "Build and serve a browsable, searchable HTML site of the wiki",
		Long: `Build a static HTML site of the wiki with Astro and Starlight, and serve it
locally with full-text search (Pagefind).

  okfw site up       build the site and serve it in the background
  okfw site down     stop the server and free its port
  okfw site build    build only (--out copies the site somewhere)
  okfw site status   report bun, the build directory and the server

Every concept becomes a page with its type, tags, lifecycle, trust tier and
staleness, a banner on a deprecated one naming its replacement, its
provenance, and the concepts that link to it. Each directory gets a page
from its index.md listing.

The site is built with bun (1.2 or later), which okfw itself does not need:
every site command checks for it first, before writing anything, and says
how to install it when it is missing. --bun or OKFW_BUN names a bun binary;
otherwise the one on PATH is used. The first build downloads Astro and
Starlight from the npm registry; later builds use bun's cache.

The build lives in your cache directory, not in the bundle, and the built
site is plain static files. Search loads its index over HTTP, so open the
site through a server (okfw site up, or any static host), not as a file.
Re-run okfw site build to refresh a running site.`,
	}
	cmd.PersistentFlags().StringVar(&bunPath, "bun", "", "Path to the bun binary (default: $"+site.BunEnv+", then bun on PATH)")

	workdir := func() (*site.Workdir, error) {
		cfg, err := config.Discover("")
		if err != nil {
			return nil, err
		}
		return site.WorkdirFor(cfg.RootPath, siteCacheDir)
	}
	ctx := func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt)
	}

	var out string
	build := &cobra.Command{
		Use:   "build",
		Short: "Build the site",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			bun, err := site.Preflight(bunPath) // first: nothing is written without bun
			if err != nil {
				return err
			}
			w, err := workdir()
			if err != nil {
				return err
			}
			c, cancel := ctx()
			defer cancel()
			stats, err := site.Build(c, w, bun, out, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			where := w.DistDir()
			if out != "" {
				where = out
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Built %d concept page(s) and %d directory page(s) into %s\n", stats.Concepts, stats.Directories, where)
			if stats.Dangling > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "%d link(s) name a concept that does not exist and were left as written.\n", stats.Dangling)
			}
			return nil
		},
	}
	build.Flags().StringVar(&out, "out", "", "Also copy the built site to this directory")

	var port int
	var noBuild bool
	up := &cobra.Command{
		Use:   "up",
		Short: "Build the site and serve it in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			bun, err := site.Preflight(bunPath)
			if err != nil {
				return err
			}
			w, err := workdir()
			if err != nil {
				return err
			}
			if s := w.Running(); s != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Already serving %s (pid %d). Run okfw site build to refresh it.\n", s.URL, s.PID)
				return nil
			}
			if err := site.PortFree(port); err != nil {
				return err
			}
			c, cancel := ctx()
			defer cancel()
			if !noBuild {
				if _, err := site.Build(c, w, bun, "", cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			s, err := site.Up(c, w, bun, port, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Serving %s (pid %d). Stop it with okfw site down.\n", s.URL, s.PID)
			return nil
		},
	}
	up.Flags().IntVar(&port, "port", 4321, "Port to serve on")
	up.Flags().BoolVar(&noBuild, "no-build", false, "Serve the last build without rebuilding")

	down := &cobra.Command{
		Use:   "down",
		Short: "Stop the site server and free its port",
		Long: `Stop the server okfw site up started. It asks Astro to stop it; if bun is
no longer available, it stops the process Astro recorded itself, once it has
confirmed that process is this wiki's preview server.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w, err := workdir()
			if err != nil {
				return err
			}
			// down must free the port even when bun has gone away, so a failed
			// preflight is a warning here, not an error.
			bun, perr := site.Preflight(bunPath)
			if perr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %v\nStopping the server without bun.\n", perr)
				bun = nil
			}
			c, cancel := ctx()
			defer cancel()
			s, err := site.Down(c, w, bun)
			if err != nil {
				return err
			}
			if s == nil {
				fmt.Fprintln(cmd.OutOrStdout(), "No site server is running.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Stopped %s (pid %d); port %d is free.\n", s.URL, s.PID, s.Port)
			return nil
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Report bun, the build directory and the site server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			o := cmd.OutOrStdout()
			bun, perr := site.Preflight(bunPath)
			if perr != nil {
				fmt.Fprintf(o, "bun:      not usable — %v\n", perr)
			} else {
				fmt.Fprintf(o, "bun:      %s %s (from %s)\n", bun.Path, bun.Version, bun.Source)
			}
			w, err := workdir()
			if err != nil {
				return err
			}
			fmt.Fprintf(o, "build:    %s\n", w.Dir)
			if _, err := os.Stat(w.DistDir()); err == nil {
				fmt.Fprintf(o, "site:     %s\n", w.DistDir())
			} else {
				fmt.Fprintln(o, "site:     not built yet (okfw site build)")
			}
			if s := w.Running(); s != nil {
				fmt.Fprintf(o, "server:   %s (pid %d, since %s)\n", s.URL, s.PID, s.StartedAt)
			} else {
				fmt.Fprintln(o, "server:   not running")
			}
			if perr != nil {
				return perr
			}
			return nil
		},
	}

	cmd.AddCommand(build, up, down, status)
	return cmd
}
