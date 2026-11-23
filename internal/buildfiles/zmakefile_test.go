package buildfiles

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	phonyRe      = regexp.MustCompile(`(?m)^\.PHONY:(.*)$`)
	zmakeTaskRe  = regexp.MustCompile(`(?m)^@([a-z][a-z0-9-]*):`)
	staticRuleRe = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)+): [a-z0-9%-]+:`)
	ruleHeadRe   = regexp.MustCompile(`(?m)^([a-z][a-z0-9-]*):(?:[^=]|$)`)
)

func read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestZMakefileMatchesMakefile: zmake reads the ZMakefile before the
// Makefile, so the two must offer the same tasks. The Makefile's tasks are its
// .PHONY list, every one of which must have a rule; the ZMakefile's are its
// @name: tasks plus the targets of its static pattern rule. `make
// zmake-check`, run in CI, compares the recipes.
func TestZMakefileMatchesMakefile(t *testing.T) {
	mk, zmk := read(t, "Makefile"), read(t, "ZMakefile")

	m := phonyRe.FindStringSubmatch(mk)
	if m == nil {
		t.Fatal("the Makefile has no .PHONY list")
	}
	makeTasks := map[string]bool{}
	for _, name := range strings.Fields(m[1]) {
		makeTasks[name] = true
	}
	ruled := map[string]bool{}
	for _, r := range ruleHeadRe.FindAllStringSubmatch(mk, -1) {
		ruled[r[1]] = true
	}
	for _, r := range staticRuleRe.FindAllStringSubmatch(mk, -1) {
		for _, name := range strings.Fields(r[1]) {
			ruled[name] = true
		}
	}
	for name := range makeTasks {
		if !ruled[name] {
			t.Errorf("Makefile: %s is in .PHONY but has no rule", name)
		}
	}

	zmakeTasks := map[string]bool{}
	for _, r := range zmakeTaskRe.FindAllStringSubmatch(zmk, -1) {
		zmakeTasks[r[1]] = true
	}
	for _, r := range staticRuleRe.FindAllStringSubmatch(zmk, -1) {
		for _, name := range strings.Fields(r[1]) {
			zmakeTasks[name] = true
		}
	}
	if phonyRe.MatchString(zmk) {
		t.Error("the ZMakefile declares .PHONY; modern mode uses @name: tasks")
	}
	if got, want := sorted(zmakeTasks), sorted(makeTasks); !reflect.DeepEqual(got, want) {
		t.Errorf("tasks differ:\nMakefile:  %v\nZMakefile: %v", want, got)
	}

	// zmake-check compares every task's recipe; a task left out of its list is
	// a task whose recipe can drift.
	listRe := regexp.MustCompile(`(?s)ZMAKE_CHECK_TASKS=(.*?)\n[a-zA-Z@]`)
	for name, src := range map[string]string{"Makefile": mk, "ZMakefile": zmk} {
		lm := listRe.FindStringSubmatch(src)
		if lm == nil {
			t.Fatalf("%s has no ZMAKE_CHECK_TASKS", name)
		}
		listed := map[string]bool{"zmake-check": true}
		for _, w := range strings.Fields(strings.ReplaceAll(lm[1], "\\", " ")) {
			listed[w] = true
		}
		if got, want := sorted(listed), sorted(makeTasks); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ZMAKE_CHECK_TASKS %v does not cover every task %v", name, got, want)
		}
	}
}
