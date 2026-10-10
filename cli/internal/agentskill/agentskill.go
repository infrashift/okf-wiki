// Package agentskill installs a tool's agent integration for the three
// supported platforms: Claude Code, Gemini CLI, and Codex CLI.
//
// It holds the platform mechanics — where a SKILL.md lives and how a managed
// block is merged into CLAUDE.md/GEMINI.md/AGENTS.md — apart from the content,
// which lives in okfwskills.
//
// All prose insertions use managed marker blocks so re-install is idempotent
// and never clobbers user content.
package agentskill

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// Spec is one tool's agent-facing documentation, already rendered.
type Spec struct {
	// Name drives the marker block and the .claude/skills/<name>/ directory.
	Name string
	// Skill is the SKILL.md body. Claude Code is the only platform that takes
	// a separate skill file; the others get Instructions only.
	Skill string
	// Instructions is the block merged into CLAUDE.md / GEMINI.md / AGENTS.md.
	// It rides in every agent context, so it must stay short.
	Instructions string
}

// Options configures an install.
type Options struct {
	Root     string // repo root
	Platform string // claude | gemini | codex
	Force    bool   // rewrite the skill file even if it exists
}

// Result reports what happened.
type Result struct {
	Written []string // files created or updated
	Notes   []string // follow-up guidance printed to the user
}

// Install writes the platform integration for one tool.
func Install(spec Spec, opts Options) (*Result, error) {
	res := &Result{}

	switch opts.Platform {
	case "claude":
		if spec.Skill != "" {
			skillPath := filepath.Join(opts.Root, ".claude", "skills", spec.Name, "SKILL.md")
			wrote, err := writeFileIfNeeded(skillPath, spec.Skill, opts.Force)
			if err != nil {
				return nil, err
			}
			if wrote {
				res.Written = append(res.Written, Rel(opts.Root, skillPath))
			}
		}
		mdPath := filepath.Join(opts.Root, "CLAUDE.md")
		if err := upsertBlock(mdPath, spec.Name, spec.Instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "CLAUDE.md")

	case "gemini":
		mdPath := filepath.Join(opts.Root, "GEMINI.md")
		if err := upsertBlock(mdPath, spec.Name, spec.Instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "GEMINI.md")

	case "codex":
		mdPath := filepath.Join(opts.Root, "AGENTS.md")
		if err := upsertBlock(mdPath, spec.Name, spec.Instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "AGENTS.md")

	default:
		return nil, fmt.Errorf("unknown platform %q (supported: claude, gemini, codex)", opts.Platform)
	}
	return res, nil
}

// Render executes an embedded asset template against a repository name.
func Render(tmpl, repo string) (string, error) {
	t, err := template.New("asset").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, struct{ Repo string }{Repo: repo}); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Rel renders a path relative to the repo root, for reporting.
func Rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}

// writeFileIfNeeded creates the file (and parents); existing files are left
// alone unless force is set. Reports whether it wrote.
func writeFileIfNeeded(path, content string, force bool) (bool, error) {
	if _, err := os.Stat(path); err == nil && !force {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(content), 0644)
}

// upsertBlock inserts or replaces one tool's managed block in a markdown file,
// creating the file when missing. The markers are namespaced by tool, so the
// block coexists with user content and with other tools' blocks.
func upsertBlock(path, name, body string) error {
	begin := "<!-- BEGIN " + name + " -->"
	end := "<!-- END " + name + " -->"
	block := begin + "\n" + strings.TrimSpace(body) + "\n" + end

	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(block+"\n"), 0644)
	}
	if err != nil {
		return err
	}
	text := string(content)
	b := strings.Index(text, begin)
	e := strings.Index(text, end)
	if b >= 0 && e > b {
		text = text[:b] + block + text[e+len(end):]
	} else {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + block + "\n"
	}
	return os.WriteFile(path, []byte(text), 0644)
}
