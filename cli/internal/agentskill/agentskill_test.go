package agentskill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testSpec = Spec{
	Name:         "tool",
	Skill:        "---\nname: tool\n---\n\nUse tool.\n",
	Instructions: "Run `tool help` first.",
}

func TestInstallClaude(t *testing.T) {
	root := t.TempDir()
	res, err := Install(testSpec, Options{Root: root, Platform: "claude"})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "tool", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md: %v", err)
	}
	if string(skill) != testSpec.Skill {
		t.Errorf("SKILL.md = %q, want %q", skill, testSpec.Skill)
	}
	md, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- BEGIN tool -->\nRun `tool help` first.\n<!-- END tool -->"
	if !strings.Contains(string(md), want) {
		t.Errorf("CLAUDE.md block missing:\n%s", md)
	}
	if strings.Join(res.Written, ",") != ".claude/skills/tool/SKILL.md,CLAUDE.md" {
		t.Errorf("Written = %v", res.Written)
	}
}

func TestInstallPreservesUserContent(t *testing.T) {
	root := t.TempDir()
	userText := "# My project\n\nHand-written instructions.\n"
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(userText), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(testSpec, Options{Root: root, Platform: "claude"}); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.HasPrefix(string(md), userText) {
		t.Errorf("user CLAUDE.md content clobbered:\n%s", md)
	}
}

func TestInstallIsIdempotentAndReplacesTheBlock(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := Install(testSpec, Options{Root: root, Platform: "claude"}); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
	}
	updated := testSpec
	updated.Instructions = "Run `tool doctor` first."
	if _, err := Install(updated, Options{Root: root, Platform: "claude"}); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if strings.Count(string(md), "<!-- BEGIN tool -->") != 1 {
		t.Errorf("duplicated blocks:\n%s", md)
	}
	if !strings.Contains(string(md), "tool doctor") || strings.Contains(string(md), "tool help") {
		t.Errorf("block not replaced in place:\n%s", md)
	}
}

func TestInstallKeepsAnExistingSkillUnlessForced(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".claude", "skills", "tool", "SKILL.md")
	if _, err := Install(testSpec, Options{Root: root, Platform: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(testSpec, Options{Root: root, Platform: "claude"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "edited" {
		t.Errorf("unforced install overwrote the skill: %q", got)
	}
	if _, err := Install(testSpec, Options{Root: root, Platform: "claude", Force: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != testSpec.Skill {
		t.Errorf("forced install did not rewrite the skill: %q", got)
	}
}

func TestInstallGeminiAndCodex(t *testing.T) {
	for platform, file := range map[string]string{"gemini": "GEMINI.md", "codex": "AGENTS.md"} {
		root := t.TempDir()
		if _, err := Install(testSpec, Options{Root: root, Platform: platform}); err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		md, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || !strings.Contains(string(md), "<!-- BEGIN tool -->") {
			t.Errorf("%s: %s missing block: %v", platform, file, err)
		}
		if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
			t.Errorf("%s: wrote Claude-only files", platform)
		}
	}
}

func TestInstallUnknownPlatform(t *testing.T) {
	_, err := Install(testSpec, Options{Root: t.TempDir(), Platform: "cursor"})
	if err == nil || !strings.Contains(err.Error(), "unknown platform") {
		t.Fatalf("err = %v, want unknown platform", err)
	}
}
