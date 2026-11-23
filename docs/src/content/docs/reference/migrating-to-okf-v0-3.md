---
title: Migrating to OKF v0.3
description: What changed between OKF v0.2 and v0.3 for an okfw bundle, and how okfw migrate moves one across.
---

OKF v0.3 is a pre-1.0 minor version, and the spec lists two breaking changes in
it (§13.1). Both change how a consumer reads values a v0.2 bundle already
holds. okfw reads either version. A bundle that still declares `okf_version:
"0.2"` keeps working, through the compatibility fallback the spec allows.
Migrating removes the need for that fallback.

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

**Bare path fields gain a leading `/`.**
- **What changed:** okfw v0.2 read a frontmatter path without a leading `/`,
  such as `executor.resource: skills/run.md`, from the bundle root. v0.3 reads
  it from the concept's own directory (§6.2).
- **What migrate does:** if the path names a file from the bundle root and
  none from the concept's directory, migrate prefixes it with `/`. URLs,
  `./` and `../` paths, and scope descriptors are left alone. A bare `.md`
  path that names a file from neither place is reported.

```yaml
# before (okf_version "0.2")       # after (okf_version "0.3")
executor:                          executor:
  resource: skills/run.md            resource: /skills/run.md
```

This step runs only while the bundle still declares 0.2 or no version. The
bundle's version is what tells a reader how to resolve a bare path.

**`okf_version` becomes `"0.3"`.** Every other key in the root `index.md`
frontmatter is kept.

**The export scratch directory moves** from `ephemeral/` to `.ephemeral/`, and
`.okfw/.gitignore` is updated to match. v0.3 treats only dot-named paths as
outside the bundle. Under the old name, another OKF tool would read every
exported copy as a second concept.

## What migrate reports, and you fix

- **A missing bundle `title` or `description`.** okfw requires both (see
  [where we differ](/okf-wiki/reference/okf-divergences/)). Set them with
  `okfw set-bundle-meta`.
- **A `verified` event without an `at`.** v0.3 requires both `by` and `at`
  in an event and ignores one that lacks either. Run `okfw verify` again after
  checking the concept.
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
that day in UTC, so okfw accepts it and leaves it as written. A `stale_after`
that cannot be parsed now counts as stale, never as fresh.

**New:**
- `superseded_by` names the replacement of a deprecated concept:
  `okfw set-lifecycle <ref> deprecated --superseded-by <new>`.
- `okfw search --current` filters to current knowledge and follows
  `superseded_by` to the replacement.
- Custom metadata goes in `x-<producer>-<name>` keys, at any level.
- A `.md` file with no `type` is not a concept, so a `README.md` in the bundle
  is allowed.
