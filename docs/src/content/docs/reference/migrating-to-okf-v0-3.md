---
title: Migrating to OKF v0.3
description: What changed between OKF v0.2 and v0.3 for an okfw bundle, and how okfw migrate moves one across.
---

OKF v0.3 is a pre-1.0 minor version, and the spec lists two breaking changes in
it (§13.1). Both change how a consumer reads values a v0.2 bundle already
holds. okfw reads either version. A bundle that still declares `okf_version:
"0.2"` keeps working, through the compatibility fallback the spec allows.
After a migration, the bundle does not need that fallback.

## Migrate

```sh
okfw migrate --dry-run   # report what would change
okfw migrate             # apply it
okfw set-bundle-meta --description "What this bundle covers and when to use it."
okfw validate --all
```

`okfw migrate` is idempotent: a second run reports nothing to do. Commit
before you run it, so `git diff` shows exactly what changed.

## What migrate changes

**okfw's own keys move to `x-okfw-*`.** `id`, `kind`, `project`, `parent`,
`created` and `timestamp` become `x-okfw-id`, `x-okfw-kind`, `x-okfw-project`,
`x-okfw-parent`, `x-okfw-created` and `x-okfw-timestamp`. Each key keeps its
place, value and quoting. This puts them in the namespace that OKF v0.3 reserves for
producers (§4.1), where no future OKF version can redefine them. migrate changes
only the concepts that okfw wrote: those with an okfw-shaped id, or with both
`created` and `timestamp`. migrate does not change a bare `id` that another tool
wrote. okfw still reads the old names, but `okfw validate` reports them until
you migrate.

**Bare path fields gain a leading `/`.**
- **What changed:** okfw v0.2 read a frontmatter path without a leading `/`,
  such as `executor.resource: skills/run.md`, from the bundle root. v0.3 reads
  it from the concept's own directory (§6.2).
- **What migrate does:** it looks for the file from the bundle root and from the
  concept's directory. When only the bundle root has it, migrate puts `/` in
  front of the path. migrate
  does not change URLs, `./` and `../` paths, or scope descriptors. It reports
  a bare `.md` path that names a file from neither place.

```yaml
# before (okf_version "0.2")       # after (okf_version "0.3")
executor:                          executor:
  resource: skills/run.md            resource: /skills/run.md
```

This step runs only while the bundle still declares 0.2 or no version. The
bundle's version is what tells a reader how to resolve a bare path.

**`okf_version` becomes `"0.3"`.** migrate keeps each other key in the root
`index.md` frontmatter.

**The export scratch directory moves** from `ephemeral/` to `.ephemeral/`, and
migrate changes `.okfw/.gitignore` to match. v0.3 treats only dot-named paths
as outside the bundle. Under the old name, another OKF tool reads each
exported copy as a second concept.

## What migrate reports, and you fix

- **A bundle without a `title` or a `description`.** okfw requires both (see
  [where we differ](/okf-wiki/reference/okf-divergences/)). Set them with
  `okfw set-bundle-meta`.
- **A `verified` event without an `at`.** v0.3 requires both `by` and `at`
  in an event and ignores one that lacks either. Check the concept, then run
  `okfw verify` again.
- **A timestamp in neither v0.3 form**, for example a date-time with no
  offset. Only its author knows which instant it meant.

## What changes without a rewrite

**Trust tiers count only current reviews.** A `verified` event counts only if
its `at` is at or after `generated.at`. This is the second breaking change.
A concept whose content changed after its last review now reads as
unverified, or as machine-confirmed if a machine check is still current.
Check the concept, then run `okfw verify`. The tier can only go down, which
is the safe direction.

Since v0.3, okfw moves `generated.at` only when the content changes: through
`okfw set-generated`, or a body rewrite through okfw. Tag, lifecycle, source
and verify edits no longer move it, so they no longer invalidate a review.

**Bare dates are valid again.** `stale_after: 2026-12-31` means the start of
that day in UTC, so okfw accepts it and keeps it as written. A `stale_after`
that okfw cannot parse now counts as stale, never as fresh.

**New:**
- `superseded_by` names the concept that replaces a deprecated one:
  `okfw set-lifecycle <ref> deprecated --superseded-by <new>`.
- `okfw search --current` shows only current knowledge. For a deprecated hit,
  it gives the concept in `superseded_by`.
- Custom metadata goes in `x-<producer>-<name>` keys, at any level.
- A `.md` file with no `type` is not a concept, so a bundle can hold a
  `README.md`.
