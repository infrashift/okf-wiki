package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallClaude(t *testing.T) {
	root := t.TempDir()
	res, err := Install(Options{Root: root, Platform: "claude", Hooks: true})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	skill, err := os.ReadFile(filepath.Join(root, ".claude", "skills", "graphy", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md: %v", err)
	}
	if !strings.Contains(string(skill), "name: graphy") || !strings.Contains(string(skill), "graphy query") {
		t.Errorf("SKILL.md content:\n%s", skill)
	}

	md, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "<!-- BEGIN graphy -->") || !strings.Contains(string(md), "graphy update") {
		t.Errorf("CLAUDE.md block missing:\n%s", md)
	}

	var settings map[string]interface{}
	raw, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("settings.json invalid: %v\n%s", err, raw)
	}
	if !strings.Contains(string(raw), "graphy update") {
		t.Errorf("hook missing:\n%s", raw)
	}
	if len(res.Written) == 0 {
		t.Errorf("no files reported written")
	}
}

func TestInstallPreservesUserContent(t *testing.T) {
	root := t.TempDir()
	userText := "# My project\n\nHand-written instructions.\n"
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(userText), 0644); err != nil {
		t.Fatal(err)
	}
	userSettings := `{"permissions": {"allow": ["Bash(go test:*)"]}}`
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(userSettings), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(Options{Root: root, Platform: "claude", Hooks: true}); err != nil {
		t.Fatal(err)
	}

	md, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(md), "Hand-written instructions.") {
		t.Errorf("user CLAUDE.md content clobbered:\n%s", md)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if !strings.Contains(string(raw), "go test") {
		t.Errorf("user settings clobbered:\n%s", raw)
	}
	if !strings.Contains(string(raw), "graphy update") {
		t.Errorf("hook not merged:\n%s", raw)
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 3; i++ {
		if _, err := Install(Options{Root: root, Platform: "claude", Hooks: true}); err != nil {
			t.Fatalf("install %d: %v", i, err)
		}
	}
	md, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if strings.Count(string(md), "<!-- BEGIN graphy -->") != 1 {
		t.Errorf("duplicated blocks:\n%s", md)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if strings.Count(string(raw), "graphy update") != 1 {
		t.Errorf("duplicated hooks:\n%s", raw)
	}
}

func TestInstallGeminiAndCodex(t *testing.T) {
	for platform, file := range map[string]string{"gemini": "GEMINI.md", "codex": "AGENTS.md"} {
		root := t.TempDir()
		res, err := Install(Options{Root: root, Platform: platform, Hooks: true})
		if err != nil {
			t.Fatalf("%s: %v", platform, err)
		}
		md, err := os.ReadFile(filepath.Join(root, file))
		if err != nil || !strings.Contains(string(md), "<!-- BEGIN graphy -->") {
			t.Errorf("%s: %s missing block: %v", platform, file, err)
		}
		if len(res.Notes) == 0 {
			t.Errorf("%s: --hooks should produce guidance notes", platform)
		}
	}
}

func TestInstallUnknownPlatform(t *testing.T) {
	if _, err := Install(Options{Root: t.TempDir(), Platform: "cursor"}); err == nil {
		t.Fatal("expected error for unsupported platform")
	}
}
