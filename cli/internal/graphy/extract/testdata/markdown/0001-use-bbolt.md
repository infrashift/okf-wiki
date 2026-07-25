# Use bbolt for the graph store

## Context

We considered `Neo4j` and Postgres AGE, but both add a server dependency
that a single-binary CLI cannot justify.

## Decision

Use `bbolt`: embedded, pure Go, single file, read-optimized. The write path
goes through `Store.PutFile`.

## Consequences

Queries are hand-written rather than Cypher.
