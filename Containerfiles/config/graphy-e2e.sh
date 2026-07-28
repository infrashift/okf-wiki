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

# Graph queries read the concepts on disk. There is no store to create, so the
# only thing to assert is the absence: a query must leave no derived state
# behind. The removed bbolt store made this a live hazard — bolt.Open created
# the file, and every command choosing between store and scan by whether it
# existed then answered from the empty store, permanently, after one query.
okfw graph-query dangling >/dev/null 2>&1 || true
[ ! -e .okfw/.graph.db ] || fail "a graph query created a graph database"
[ ! -e .okfw/.index ] || fail "a graph query created a search index"
pass "graph queries create no derived state"
# --min-degree 1 on a fixture this small: the default of 3 would export only the
# few hub entities, leaving the bridge and traversal checks below with nothing
# to resolve. It also exercises a non-default threshold.
graphy export okfw --min-degree 1 >export.log 2>&1 || {
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

# The exported tree — one `parent:` edge per concept — must be traversable
# immediately. It is read from the files, so no sync step stands between the
# export and okfw graph, backlinks or graph-query.
okfw graph-query dangling >/dev/null || fail "okfw graph-query cannot read the exported tree"
[ ! -e .okfw/.graph.db ] || fail "the export created a graph database"
pass "the exported tree is traversable with no sync step"

# Code concepts must be separable from hand-written ones. Every exported concept
# used to index as kind "note", so an agent asking for its notes got the whole
# code export as well, with no way to exclude it.
okfw search --kind code --json >code.json 2>&1 || fail "okfw search --kind code"
grep -q '"title"' code.json || {
	cat code.json >&2
	fail "no concept is indexed as kind code"
}
okfw search --kind note --json >notes.json 2>&1 || fail "okfw search --kind note"
grep -q '"title"' notes.json && {
	cat notes.json >&2
	fail "a generated concept is still indexed as kind note"
}
pass "graphy's concepts are separable from hand-written notes (--kind code/note)"

# A repo-relative path resolves to the file node, which is never exported — so
# this used to fail with advice ("run graphy export okfw") that could never work.
graphy concept src/server.go --json >filecpt.json 2>&1 || fail "graphy concept <file path>"
grep -q "Greet" filecpt.json || {
	cat filecpt.json >&2
	fail "graphy concept <file path> did not list the entities documented in it"
}
pass "graphy concept answers for a repo-relative file path"

# ── A second export must be a no-op. This is graphy's idempotency contract, and
#    it is easy to break: any stamp that moves on every run defeats it.
graphy export okfw --min-degree 1 >export2.log 2>&1 || {
	cat export2.log >&2
	fail "second graphy export okfw"
}
grep -qE '0 created, 0 updated' export2.log || {
	cat export2.log >&2
	fail "re-export was not a no-op"
}
pass "re-exporting an unchanged graph changes nothing"

# ── The bridge between the two binaries. graphy knows node ids, okfw knows
#    concept ids, and only the export records the correspondence — so this is
#    the one place the round trip can be checked against both real binaries.
node_id="$(graphy concept Greet --json | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')"
[ -n "$node_id" ] || fail "graphy concept returned no node id for Greet"

concept_path="$(graphy concept Greet --json | sed -n 's/.*"abs_path":"\([^"]*\)".*/\1/p')"
[ -n "$concept_path" ] || fail "graphy concept returned no absolute path (was the bundle recorded?)"
[ -f "$concept_path" ] || fail "graphy concept points at $concept_path, which does not exist"
pass "graphy concept resolves a code entity to a readable concept file"

# The concept must carry the node id back, or the reverse direction has nothing
# to match on.
grep -q "node_id: \"$node_id\"" "$concept_path" ||
	fail "the concept does not record the node id it was generated from"

# And okfw must find it by that id — which only works if the id reached the
# search index as a keyword field.
okfw search --node-id "$node_id" --json >bridge.json 2>&1 ||
	fail "okfw search --node-id failed"
grep -q '"title":"Greet"' bridge.json || grep -q '"title": "Greet"' bridge.json || {
	cat bridge.json >&2
	fail "okfw could not find the concept for $node_id"
}
pass "the node id round-trips: graphy concept -> okfw search --node-id"

# The bridge cannot be broken by losing derived state, because there is none to
# lose. This used to be the sharpest edge in the seam: a re-export found every
# concept byte-identical and wrote nothing, so a lost .okfw/.index left
# `okfw search --node-id` returning zero hits for good.
graphy export okfw --min-degree 1 >export3.log 2>&1 || {
	cat export3.log >&2
	fail "re-export"
}
grep -qE '0 created, 0 updated' export3.log || {
	cat export3.log >&2
	fail "the no-op re-export rewrote concepts"
}
okfw search --node-id "$node_id" --json >bridge2.json 2>&1 ||
	fail "okfw search --node-id failed after a no-op re-export"
grep -q "Greet" bridge2.json || {
	cat bridge2.json >&2
	fail "the node-id bridge did not survive a no-op re-export"
}
pass "the node-id bridge survives a no-op re-export"

# okfw move cannot update graphy'"'"'s exports bucket, so the next export used to
# create a duplicate: two concepts carrying the same node id.
okfw move "$concept_path" archive/ >/dev/null || fail "okfw move"
graphy export okfw --min-degree 1 >export4.log 2>&1 || {
	cat export4.log >&2
	fail "re-export after okfw move"
}
grep -qE '0 created' export4.log || {
	cat export4.log >&2
	fail "the moved concept was duplicated rather than adopted"
}
okfw search --node-id "$node_id" --json >bridge3.json 2>&1 || fail "okfw search --node-id after move"
[ "$(grep -o '"id"' bridge3.json | wc -l)" -eq 1 ] || {
	cat bridge3.json >&2
	fail "node id $node_id is on more than one concept"
}
pass "a concept moved by okfw is adopted, not duplicated"

# ── Deterministic traversal. These answer impact questions without a model, so
#    a missing edge here is a silently wrong answer rather than a crash.
graphy callers Listen --json >callers.json 2>&1 || fail "graphy callers"
grep -q "Server.Start" callers.json || {
	cat callers.json >&2
	fail "graphy callers did not find Server.Start calling Listen"
}
pass "graphy callers answers the impact question"

graphy nodes --file src/server.go --json >nodes.json 2>&1 || fail "graphy nodes --file"
grep -q "Greet" nodes.json || fail "graphy nodes --file listed no entities"
pass "graphy nodes --file maps a path into the graph"

graphy why Greet >why.txt 2>&1 || fail "graphy why"
grep -q "WHY" why.txt || {
	cat why.txt >&2
	fail "graphy why did not surface the WHY: comment"
}
pass "graphy why surfaces recorded rationale"

# ── Interchange formats: none may stamp a timestamp, so an unchanged graph has
#    to serialize byte for byte.
for fmt in json graphml jgf cypher; do
	graphy export "$fmt" >/dev/null 2>&1 || fail "graphy export $fmt"
done
pass "all four interchange formats export"

echo
echo "=== graphy e2e: all checks passed ==="
