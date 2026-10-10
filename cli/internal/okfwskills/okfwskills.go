// Package okfwskills holds okfw's agent-facing documentation and installs it
// via the shared agentskill package.
//
// okfw once shipped no installer at all: the skill was a loose file at the repo
// root that README told the reader to copy into .claude/skills/ by hand, with
// no CLAUDE.md block and nothing checking it against the commands okfw
// actually registers.
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
// No hook: search and the link graph are computed from the files, and okfw's
// own writers refresh the index.md listings, so there is nothing for an
// after-edit hook to refresh.
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
