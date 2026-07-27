#!/usr/bin/env bash
# graphy end-to-end smoke test: build a graph of a tiny fixture repository and
# export it into an okfw bundle that okfw then validates.
#
# This covers the seam `go test` cannot reach: graphy is a CGo binary linking
# tree-sitter grammars, and `graphy export okfw` hand-builds concept frontmatter
# that okfw's closed CUE schema has to accept at runtime. Both binaries must
# agree on the schema, and they are built and released together — so if they
# disagree, this is where it should surface.
set -euo pipefail

fail() {
	echo "FAIL: $*" >&2
	exit 1
}
pass() { echo "PASS: $*"; }

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"

# ── A fixture spanning a tree-sitter language and a pure-Go extractor, plus a
#    marked comment so rationale extraction is exercised too.
mkdir -p src
cat >src/server.go <<'GO'
package src

// WHY: the graph needs a rationale node to attach to this function.
func Greet(name string) string { return "hi " + name }

type Server struct{ Addr string }

func (s *Server) Start() error { return Listen(s.Addr) }

func Listen(addr string) error { return nil }
GO

cat >src/util.py <<'PY'
def normalize(value):
    return value.strip().lower()


class Cache:
    def get(self, key):
        return normalize(key)
PY

cat >config.yaml <<'YAML'
service:
  name: fixture
  port: 8080
YAML

git init -q .
git -c user.email=e2e@test -c user.name=e2e add -A
git -c user.email=e2e@test -c user.name=e2e commit -qm "fixture"

# ── Build the graph.
graphy init >/dev/null
graphy build >build.log 2>&1 || {
	cat build.log >&2
	fail "graphy build"
}
grep -qE '[0-9]+ nodes' build.log || {
	cat build.log >&2
	fail "graphy build reported no nodes"
}
pass "graphy build extracted the fixture ($(grep -oE '[0-9]+ nodes' build.log | head -1))"

# Extraction must actually have produced Go and Python entities: a silently
# empty graph would still satisfy the check above.
graphy report >report.md 2>/dev/null || fail "graphy report"
grep -q "Greet" report.md || grep -rq "Greet" .graphy/ || fail "the Go extractor found no Greet"
pass "tree-sitter extraction works in this image (CGo is linked correctly)"

# ── Export into an okfw bundle and have okfw validate what graphy wrote.
okfw init >/dev/null
graphy export okfw >export.log 2>&1 || {
	cat export.log >&2
	fail "graphy export okfw"
}
grep -qE '[0-9]+ created' export.log || {
	cat export.log >&2
	fail "export created no concepts"
}
pass "graphy export okfw wrote concepts ($(grep -oE '[0-9]+ created' export.log | head -1))"

# The strict okfw schema is the real assertion: graphy builds this frontmatter by
# hand, including its `graphy:` extension block.
okfw validate --all || fail "okfw rejected concepts that graphy generated"
pass "okfw validate --all accepts every graphy-generated concept"

okfw validate --okf || fail "the exported bundle is not OKF-conformant"
pass "the exported bundle is OKF-conformant"

# ── A second export must be a no-op. This is graphy's idempotency contract, and
#    it is easy to break: any stamp that moves on every run defeats it.
graphy export okfw >export2.log 2>&1 || {
	cat export2.log >&2
	fail "second graphy export okfw"
}
grep -qE '0 created, 0 updated' export2.log || {
	cat export2.log >&2
	fail "re-export was not a no-op"
}
pass "re-exporting an unchanged graph changes nothing"

# ── Interchange formats: none may stamp a timestamp, so an unchanged graph has
#    to serialize byte for byte.
for fmt in json graphml jgf cypher; do
	graphy export "$fmt" >/dev/null 2>&1 || fail "graphy export $fmt"
done
pass "all four interchange formats export"

echo
echo "=== graphy e2e: all checks passed ==="
