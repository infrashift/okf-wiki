// Package skills installs graphy's agent integration for the three supported
// platforms: Claude Code, Gemini CLI, and Codex CLI.
//
// All prose insertions use managed marker blocks so re-install is idempotent
// and never clobbers user content; the Claude hook merges into
// .claude/settings.json preserving existing keys.
package skills

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed assets/SKILL.md.tmpl
var skillTmpl string

//go:embed assets/instructions.md.tmpl
var instructionsTmpl string

const (
	beginMarker = "<!-- BEGIN graphy -->"
	endMarker   = "<!-- END graphy -->"
)

// Options configures an install.
type Options struct {
	Root     string // repo root
	Platform string // claude | gemini | codex
	Hooks    bool   // also install an after-edit hook where supported
	Force    bool   // rewrite the skill file even if it exists
}

// Result reports what happened.
type Result struct {
	Written []string // files created or updated
	Notes   []string // follow-up guidance printed to the user
}

// Install writes the platform integration.
func Install(opts Options) (*Result, error) {
	repo := filepath.Base(opts.Root)
	instructions, err := render(instructionsTmpl, repo)
	if err != nil {
		return nil, err
	}
	res := &Result{}

	switch opts.Platform {
	case "claude":
		skill, err := render(skillTmpl, repo)
		if err != nil {
			return nil, err
		}
		skillPath := filepath.Join(opts.Root, ".claude", "skills", "graphy", "SKILL.md")
		wrote, err := writeFileIfNeeded(skillPath, skill, opts.Force)
		if err != nil {
			return nil, err
		}
		if wrote {
			res.Written = append(res.Written, rel(opts.Root, skillPath))
		}
		mdPath := filepath.Join(opts.Root, "CLAUDE.md")
		if err := upsertBlock(mdPath, instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "CLAUDE.md")
		if opts.Hooks {
			settingsPath := filepath.Join(opts.Root, ".claude", "settings.json")
			changed, err := mergeClaudeHook(settingsPath)
			if err != nil {
				return nil, err
			}
			if changed {
				res.Written = append(res.Written, rel(opts.Root, settingsPath))
			} else {
				res.Notes = append(res.Notes, "hook already present in .claude/settings.json")
			}
		}

	case "gemini":
		mdPath := filepath.Join(opts.Root, "GEMINI.md")
		if err := upsertBlock(mdPath, instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "GEMINI.md")
		if opts.Hooks {
			res.Notes = append(res.Notes,
				"Gemini CLI hook schemas vary by version — add a before/after-edit hook running `graphy update` to .gemini/settings.json manually.")
		}

	case "codex":
		mdPath := filepath.Join(opts.Root, "AGENTS.md")
		if err := upsertBlock(mdPath, instructions); err != nil {
			return nil, err
		}
		res.Written = append(res.Written, "AGENTS.md")
		if opts.Hooks {
			res.Notes = append(res.Notes,
				"Codex hook configuration is not yet stable — instruct the agent (AGENTS.md already does) or run `graphy update` yourself after edits.")
		}

	default:
		return nil, fmt.Errorf("unknown platform %q (supported: claude, gemini, codex)", opts.Platform)
	}
	return res, nil
}

func render(tmpl, repo string) (string, error) {
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

func rel(root, path string) string {
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

// upsertBlock inserts or replaces the managed graphy block in a markdown
// file, creating the file when missing.
func upsertBlock(path, body string) error {
	block := beginMarker + "\n" + strings.TrimSpace(body) + "\n" + endMarker
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return os.WriteFile(path, []byte(block+"\n"), 0644)
	}
	if err != nil {
		return err
	}
	text := string(content)
	begin := strings.Index(text, beginMarker)
	end := strings.Index(text, endMarker)
	if begin >= 0 && end > begin {
		text = text[:begin] + block + text[end+len(endMarker):]
	} else {
		if !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + block + "\n"
	}
	return os.WriteFile(path, []byte(text), 0644)
}

// mergeClaudeHook adds a PostToolUse hook running `graphy update` after
// Edit/Write tools into .claude/settings.json, preserving everything else.
// Reports whether the file changed.
func mergeClaudeHook(path string) (bool, error) {
	settings := map[string]interface{}{}
	if content, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(content, &settings); err != nil {
			return false, fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	hooks, _ := settings["hooks"].(map[string]interface{})
	if hooks == nil {
		hooks = map[string]interface{}{}
	}
	post, _ := hooks["PostToolUse"].([]interface{})
	for _, entry := range post {
		if m, ok := entry.(map[string]interface{}); ok {
			if inner, ok := m["hooks"].([]interface{}); ok {
				for _, h := range inner {
					if hm, ok := h.(map[string]interface{}); ok {
						if cmd, _ := hm["command"].(string); strings.Contains(cmd, "graphy update") {
							return false, nil // already installed
						}
					}
				}
			}
		}
	}
	post = append(post, map[string]interface{}{
		"matcher": "Edit|Write",
		"hooks": []interface{}{
			map[string]interface{}{"type": "command", "command": "graphy update"},
		},
	})
	hooks["PostToolUse"] = post
	settings["hooks"] = hooks

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return false, err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(data, '\n'), 0644)
}
