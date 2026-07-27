# Sample Document

Intro text linking to [the design](./design.md) and [code](../src/main.go).

## Architecture

Some prose with an [external link](https://example.com) that is ignored.

The entry point is `Bootstrap`, which calls `store.Open()` and is described in
[main.go, at the constructor](../src/main.go#L42). Flags like `--min-degree`,
paths like `cmd/graphy/main.go` and filler words like `the` are not identifiers.

```markdown
# not a heading — inside a fence
`FencedIdentifier` must not become a reference
```

### Deep heading is not a node

Prose under a deep heading still belongs to the enclosing `Architecture` section.

## Architecture

Duplicate heading stays deduplicated.
