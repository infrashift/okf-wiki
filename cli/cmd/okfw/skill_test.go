package main

import (
	"regexp"
	"strings"
	"testing"

	"okfw/cli/internal/okfwskills"
)

// TestSkillDocumentsEveryCommand keeps the agent-facing documentation honest
// against the binary.
//
// The skill is the only description of okfw most agents ever read. It used to
// be a loose file nothing verified, so nothing would have caught it
// advertising a command that does not exist — or failing to mention one that
// does.
func TestSkillDocumentsEveryCommand(t *testing.T) {
	skill, instructions, err := okfwskills.Docs("testrepo")
	if err != nil {
		t.Fatal(err)
	}

	// Commands that carry no agent-facing capability: setup, plumbing, and the
	// generated-file regenerators an agent should never need to call.
	exempt := map[string]bool{
		"init": true, "install": true, "mcp": true, "connect": true, "root": true,
		"index-md": true, "log": true, "templates": true, "export": true,
		"migrate": true, "help": true, "completion": true,
		// Reachable through the documented `okfw_update` actions rather than by
		// name; listing all six in a skill an agent reads on every task is
		// noise, and the conventions section names the two that matter.
		"set-project": true, "set-stale-after": true, "set-resource": true,
		"set-generated": true, "add-tags": true, "remove-tags": true, "graph": true,
		"graph-query": true,
	}
	for _, c := range rootCmd().Commands() {
		name := c.Name()
		if exempt[name] {
			continue
		}
		if !strings.Contains(skill, "okfw "+name) {
			t.Errorf("SKILL.md does not mention `okfw %s`", name)
		}
	}

	// And every MCP tool, since an agent may only ever see those.
	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		if !strings.Contains(skill, name) {
			t.Errorf("SKILL.md does not mention the MCP tool %s", name)
		}
	}

	// The instructions block is injected into CLAUDE.md/AGENTS.md and rides in
	// every context, so it stays short — but it must still name the conventions
	// that change what an agent does.
	for _, must := range []string{"okfw move", "okfw delete", "okfw search"} {
		if !strings.Contains(instructions, must) {
			t.Errorf("instructions block does not mention %q", must)
		}
	}
	if lines := strings.Count(instructions, "\n"); lines > 40 {
		t.Errorf("instructions block is %d lines; it is injected into every agent context", lines)
	}
}

// TestSkillPromisesOnlyRealCommands is the other direction: every `okfw X` the
// documentation names must actually be a registered command.
func TestSkillPromisesOnlyRealCommands(t *testing.T) {
	skill, instructions, err := okfwskills.Docs("testrepo")
	if err != nil {
		t.Fatal(err)
	}
	real := map[string]bool{}
	for _, c := range rootCmd().Commands() {
		real[c.Name()] = true
		for _, a := range c.Aliases {
			real[a] = true
		}
	}
	// Backticked only: the documentation is prose, and "okfw has no task
	// fields" is a sentence, not a promise of an `okfw has` command.
	re := regexp.MustCompile("`okfw ([a-z][a-z-]*)")
	for _, doc := range []string{skill, instructions} {
		for _, m := range re.FindAllStringSubmatch(doc, -1) {
			if !real[m[1]] {
				t.Errorf("documentation promises `okfw %s`, which is not a registered command", m[1])
			}
		}
	}
}

// TestUpdateToolMatchesTheCLIWriters pins the claim in CLAUDE.md that the MCP
// `update` action set stays at parity with the CLI writers.
//
// It did not: verify, add-source, set-resource and set-generated had no MCP
// action, so two of the three OKF v0.2 metadata families — trust and
// provenance — were unreachable to an agent, while the documentation said
// otherwise.
func TestUpdateToolMatchesTheCLIWriters(t *testing.T) {
	var actions map[string]bool
	for _, tool := range mcpTools() {
		if name, _ := tool["name"].(string); name != "okfw_update" {
			continue
		}
		schema, _ := tool["inputSchema"].(map[string]interface{})
		props, _ := schema["properties"].(map[string]interface{})
		action, _ := props["action"].(map[string]interface{})
		vals, _ := action["enum"].([]string)
		actions = make(map[string]bool, len(vals))
		for _, v := range vals {
			actions[v] = true
		}
	}
	if actions == nil {
		t.Fatal("okfw_update is not declared, or its action enum is not a []string")
	}

	// Every OKF-family CLI writer needs an action. The mapping is the command
	// name with dashes as underscores.
	for _, c := range okfFamilyCommands() {
		name := strings.Fields(c.Use)[0]
		want := strings.ReplaceAll(name, "-", "_")
		if !actions[want] {
			t.Errorf("CLI writer `okfw %s` has no MCP update action %q", name, want)
		}
	}
}
