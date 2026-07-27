// Package okfwskills holds okfw's agent-facing documentation and installs it
// via the shared agentskill package.
//
// okfw shipped no installer at all: the skill was a loose file at the repo
// root that README told the reader to copy into .claude/skills/ by hand, with
// no CLAUDE.md block and nothing checking it against the commands okfw
// actually registers. graphy had all of that, so an agent given graphy got a
// procedure and an agent given okfw got five tool schemas and no conventions.
package okfwskills

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
//
// No hook: okfw's own writers keep the index, the graph and the listings in
// step on every change, so there is nothing for an after-edit hook to refresh.
// (graphy needs one because the code graph goes stale when source files change
// outside its knowledge.)
func Install(opts Options) (*Result, error) {
	skill, instructions, err := Docs(filepath.Base(opts.Root))
	if err != nil {
		return nil, err
	}
	return agentskill.Install(agentskill.Spec{
		Name:         "okfw",
		Skill:        skill,
		Instructions: instructions,
	}, opts)
}

// Docs renders the skill and the agent-instructions block for a repository,
// without writing anything. Exported so the CLI package can check both against
// the command set it registers.
func Docs(repo string) (skill, instructions string, err error) {
	if skill, err = agentskill.Render(skillTmpl, repo); err != nil {
		return "", "", err
	}
	instructions, err = agentskill.Render(instructionsTmpl, repo)
	return skill, instructions, err
}
