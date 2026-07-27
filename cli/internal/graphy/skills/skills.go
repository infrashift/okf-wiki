// Package skills holds graphy's agent-facing documentation and installs it via
// the shared agentskill package.
//
// The platform mechanics — where SKILL.md lives, how the managed block merges
// into CLAUDE.md/GEMINI.md/AGENTS.md, how the hook merges into
// .claude/settings.json — live in cli/internal/agentskill, because okfw ships
// an agent skill too and must not import a graphy package to install it. What
// stays here is what is actually graphy's: the two templates.
package skills

import (
	_ "embed"
	"path/filepath"

	"okfw/cli/internal/agentskill"
)

//go:embed assets/SKILL.md.tmpl
var skillTmpl string

//go:embed assets/instructions.md.tmpl
var instructionsTmpl string

// Options configures an install.
type Options = agentskill.Options

// Result reports what happened.
type Result = agentskill.Result

// Install writes the platform integration.
func Install(opts Options) (*Result, error) {
	skill, instructions, err := Docs(filepath.Base(opts.Root))
	if err != nil {
		return nil, err
	}
	return agentskill.Install(agentskill.Spec{
		Name:         "graphy",
		Skill:        skill,
		Instructions: instructions,
		Hook:         "graphy update",
		HookNote: "This platform's hook schema is not stable enough to write automatically — " +
			"add an after-edit hook running `graphy update` to its settings manually.",
	}, opts)
}

// Docs renders the skill and the agent-instructions block for a repository,
// without writing anything.
//
// Exported so the CLI package can check the two against the command set it
// actually registers. The skill is the only description of graphy an agent
// ever reads, and it drifting from the binary is not hypothetical: it
// advertised impact analysis for months while no command answered it.
func Docs(repo string) (skill, instructions string, err error) {
	if skill, err = agentskill.Render(skillTmpl, repo); err != nil {
		return "", "", err
	}
	instructions, err = agentskill.Render(instructionsTmpl, repo)
	return skill, instructions, err
}
