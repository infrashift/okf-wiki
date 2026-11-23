---
title: Browsing the Wiki as a Site
description: Build and serve a searchable HTML site of an OKF wiki with okfw site.
---

`okfw site` turns the wiki into a static HTML site for people to read. Each
concept and each directory is a page, and full-text search covers all of them.
okfw builds the site with [Astro](https://astro.build) and
[Starlight](https://starlight.astro.build), and serves it locally.

```sh
okfw site up      # build, then serve in the background
okfw site down    # stop the server and free its port
```

`up` prints the address (`http://localhost:4321` unless you pass `--port`).
After you edit the wiki, run `okfw site build` to refresh the site while it runs.
The server serves the new build without a restart.

In a repository with a Makefile, two targets are enough:

```make
wiki-up:
	okfw site up

wiki-down:
	okfw site down
```

## What a page shows

- **The concept's body**. A link to a concept goes to the page of that concept.
  Links to directories, `index.md` listings and bundle files (OKF assets) also
  work.
- **Its type and tags.** Both are also search filters.
- **Its trust tier** (`human-reviewed`, `machine-confirmed`, `unverified`).
  The tier counts only the reviews made since the last content change.
- **Its lifecycle status and staleness.** A deprecated concept gets a banner
  with a link to the concept in its `superseded_by`.
- **Provenance and links:** who generated it and when, and each verification.
  The page marks a verification that is older than the last change. The page
  also shows the path fields, and the concepts that link to it.

A directory page shows that directory's `index.md` listing. The home page is
the root listing, under the bundle's `title` and `description`.

## Search

The search box (or <kbd>Ctrl</kbd>+<kbd>K</kbd>) searches the full text of
each page with [Pagefind](https://pagefind.app). Pagefind makes its index when
okfw builds the site. It loads that index over HTTP. So open the site through a
server, such as `okfw site up` or a static host, not as a local file.

## Requirements: bun

okfw builds the site with [bun](https://bun.sh) 1.2 or later. okfw itself does
not need bun: only `okfw site` uses it, and every `okfw site` command checks
for it first. Without a usable bun, the command stops before it writes
anything, and it tells you what it found and how to fix it. It checks for:

- no bun on `PATH`;
- a path that does not exist or is not executable;
- a bun that does not run, or that does not answer within five seconds;
- a bun that is too old.

`--bun <path>` or `OKFW_BUN=<path>` chooses a particular bun. A path given
this way that does not work is an error: okfw does not fall back to the bun
on `PATH`. `okfw site status` shows which bun okfw found.

`okfw site down` works also when bun is gone. It asks Astro to stop the
server. If it cannot, it stops the process itself. But first it makes sure
that the process is the server of this wiki.

The first build downloads Astro and Starlight from the npm registry. Later
builds use bun's cache and work offline.

## Where it is built

okfw builds the site in the user cache directory (`~/.cache/okfw/site/<id>/`
on Linux), never in the bundle. OKF counts each file under the bundle root as
part of the bundle. A `node_modules` there puts thousands of files in front of
each OKF tool. The built site is in `dist/` in that directory. `okfw site build --out <dir>` copies it anywhere, for
example to publish it on a static host.

A large wiki keeps its sidebar small. Above 300 concepts, the sidebar lists
only the directories. You find the concepts through the directory pages and
search. Starlight puts the sidebar into each page. A list of all concepts on
each page makes the build grow with the square of the number of concepts.
