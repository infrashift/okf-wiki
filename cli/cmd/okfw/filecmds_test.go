package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

// runCmdIn is runCmd with stdin attached, for the confirmation prompt. A
// strings.Reader is not an *os.File, so the no-terminal guard stays out of the way.
func runCmdIn(t *testing.T, cmd *cobra.Command, stdin string, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// --- delete ---

func TestDeleteCmdRequiresConfirmation(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	path, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runCmdIn(t, newDeleteCmd(), "n\n", id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "Cancelled") {
		t.Errorf("expected a cancellation notice, got:\n%s", out)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("answering no must leave the file alone")
	}

	// An empty answer is also a no — the default must never destroy anything.
	if _, err := runCmdIn(t, newDeleteCmd(), "\n", id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("a bare Enter must not delete")
	}

	// Confirming does delete.
	out, err = runCmdIn(t, newDeleteCmd(), "y\n", id)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "Deleted") {
		t.Errorf("expected a deletion notice, got:\n%s", out)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the file should be gone")
	}
}

func TestDeleteCmdForceSkipsThePrompt(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	path, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	// Empty stdin: --force must not consult it.
	if _, err := runCmdIn(t, newDeleteCmd(), "", id, "--force"); err != nil {
		t.Fatalf("delete --force: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the file should be gone")
	}
}

func TestDeleteCmdReportsInboundLinks(t *testing.T) {
	cfg := bundleAt(t)
	doomedPath, doomedID, err := bundle.CreateConcept(cfg, bundle.Spec{Title: "Doomed"})
	if err != nil {
		t.Fatal(err)
	}
	doomedRel, err := filepath.Rel(cfg.RootPath, doomedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bundle.CreateConcept(cfg, bundle.Spec{
		Title: "Linker",
		Body:  "See [D](/" + filepath.ToSlash(doomedRel) + ").",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := runCmdIn(t, newDeleteCmd(), "", doomedID, "--force")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "still link to it") {
		t.Errorf("expected an inbound-link warning, got:\n%s", out)
	}
	if !strings.Contains(out, "linker--") {
		t.Errorf("expected the linking concept to be named, got:\n%s", out)
	}
}

func TestDeleteCmdRejectsUnknownRef(t *testing.T) {
	bundleAt(t)
	if _, err := runCmdIn(t, newDeleteCmd(), "", "019680f0-7e00-7abc-8def-000000000000", "--force"); err == nil {
		t.Error("expected a not-found error")
	}
}

// --- move ---

func TestMoveCmdIntoADirectory(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	oldPath, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(oldPath)

	out, err := runCmd(t, newMoveCmd(), id, "archive/")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !strings.Contains(out, "Moved") {
		t.Errorf("expected a move notice, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(cfg.RootPath, "archive", base)); err != nil {
		t.Errorf("concept not at its new home: %v", err)
	}
	if _, err := os.Stat(oldPath); err == nil {
		t.Error("the old file survived")
	}
}

// A destination with no .md suffix is a directory, as with mv.
func TestMoveCmdBareNameIsADirectory(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	oldPath, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, newMoveCmd(), id, "archive"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.RootPath, "archive", filepath.Base(oldPath))); err != nil {
		t.Errorf("expected a move into archive/: %v", err)
	}
}

func TestMoveCmdRenamesWhenGivenAMarkdownPath(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newMoveCmd(), id, "notes/better-name.md"); err != nil {
		t.Fatalf("move: %v", err)
	}
	newPath := filepath.Join(cfg.RootPath, "notes", "better-name.md")
	content, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("concept not renamed: %v", err)
	}
	z, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("renamed concept does not validate: %v", err)
	}
	if z.ID != id {
		t.Errorf("id changed on rename: %q -> %q", id, z.ID)
	}
}

// Renaming at the bundle root has no directory component to sanitize.
func TestMoveCmdRenamesAtTheBundleRoot(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	if _, err := runCmd(t, newMoveCmd(), id, "renamed.md"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.RootPath, "renamed.md")); err != nil {
		t.Errorf("expected a root-level rename: %v", err)
	}
}

func TestMoveCmdReportsRewrittenLinks(t *testing.T) {
	cfg := bundleAt(t)
	targetPath, targetID, err := bundle.CreateConcept(cfg, bundle.Spec{Title: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	targetRel, err := filepath.Rel(cfg.RootPath, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bundle.CreateConcept(cfg, bundle.Spec{
		Title: "Linker",
		Body:  "See [T](/" + filepath.ToSlash(targetRel) + ").",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, newMoveCmd(), targetID, "archive/")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !strings.Contains(out, "Rewrote links in 1 concept") {
		t.Errorf("expected a rewrite report, got:\n%s", out)
	}
}

func TestMoveCmdRejectsEscapingDestinations(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	oldPath, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}

	for _, dest := range []string{"../escaped/", "../escaped.md", ".index/", ".index/x.md", "notes/index.md"} {
		t.Run(dest, func(t *testing.T) {
			if _, err := runCmd(t, newMoveCmd(), id, dest); err == nil {
				t.Errorf("destination %q should be rejected", dest)
			}
		})
	}
	if _, err := os.Stat(oldPath); err != nil {
		t.Errorf("the source must survive a rejected move: %v", err)
	}
}

func TestMoveCmdAliases(t *testing.T) {
	for _, want := range []string{"rename", "mv"} {
		found := false
		for _, a := range newMoveCmd().Aliases {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("move should be reachable as %q", want)
		}
	}
	if newDeleteCmd().Aliases[0] != "rm" {
		t.Error("delete should be reachable as rm")
	}
}

func TestFileCommandsAreRegistered(t *testing.T) {
	want := map[string]bool{"delete": false, "move": false}
	for _, c := range fileCommands() {
		name := strings.Fields(c.Use)[0]
		if _, ok := want[name]; !ok {
			t.Errorf("unexpected command %q", name)
			continue
		}
		want[name] = true
		if c.Short == "" || c.Long == "" {
			t.Errorf("%s is missing help text", name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%q is not registered", name)
		}
	}
}

// --- resolveDestination ---

func TestResolveDestination(t *testing.T) {
	cfg := bundleAt(t)
	old := filepath.Join(cfg.RootPath, "notes", "thing--0001.md")

	for _, tc := range []struct {
		dest string
		want string
	}{
		{"archive/", filepath.Join("archive", "thing--0001.md")},
		{"archive", filepath.Join("archive", "thing--0001.md")},
		{"a/b/c", filepath.Join("a", "b", "c", "thing--0001.md")},
		{"archive/renamed.md", filepath.Join("archive", "renamed.md")},
		{"renamed.md", "renamed.md"},
	} {
		t.Run(tc.dest, func(t *testing.T) {
			got, err := resolveDestination(cfg, old, tc.dest)
			if err != nil {
				t.Fatalf("resolveDestination(%q): %v", tc.dest, err)
			}
			if want := filepath.Join(cfg.RootPath, tc.want); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}

	for _, bad := range []string{"", "..", "../out", ".index", ".todos/x.md"} {
		if _, err := resolveDestination(cfg, old, bad); err == nil {
			t.Errorf("destination %q should be rejected", bad)
		}
	}
}

// An existing directory wins over the .md-means-a-filename rule, so moving into
// a directory that happens to be named like a file still works.
func TestResolveDestinationPrefersAnExistingDirectory(t *testing.T) {
	cfg := bundleAt(t)
	odd := filepath.Join(cfg.RootPath, "weird.md")
	if err := os.MkdirAll(odd, 0755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(cfg.RootPath, "notes", "thing--0001.md")

	got, err := resolveDestination(cfg, old, "weird.md")
	if err != nil {
		t.Fatalf("resolveDestination: %v", err)
	}
	if want := filepath.Join(odd, "thing--0001.md"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- --json output (what the Neovim plugin consumes instead of screen-scraping) ---

func TestDeleteCmdJSON(t *testing.T) {
	cfg := bundleAt(t)
	doomedPath, doomedID, err := bundle.CreateConcept(cfg, bundle.Spec{Title: "Doomed"})
	if err != nil {
		t.Fatal(err)
	}
	doomedRel, err := filepath.Rel(cfg.RootPath, doomedPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bundle.CreateConcept(cfg, bundle.Spec{
		Title: "Linker",
		Body:  "See [D](/" + filepath.ToSlash(doomedRel) + ").",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := runCmdIn(t, newDeleteCmd(), "", doomedID, "--force", "--json")
	if err != nil {
		t.Fatalf("delete --json: %v", err)
	}
	var got struct {
		ID      string   `json:"id"`
		Path    string   `json:"path"`
		Inbound []string `json:"inbound"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if got.ID != doomedID {
		t.Errorf("id = %q, want %q", got.ID, doomedID)
	}
	if got.Path != doomedPath {
		t.Errorf("path = %q, want the absolute path %q", got.Path, doomedPath)
	}
	if len(got.Inbound) != 1 || !strings.HasPrefix(got.Inbound[0], "linker--") {
		t.Errorf("inbound = %v, want the one linker", got.Inbound)
	}
}

// A nil slice would marshal as null; consumers must always be able to iterate.
func TestDeleteCmdJSONInboundIsNeverNull(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	out, err := runCmdIn(t, newDeleteCmd(), "", id, "--force", "--json")
	if err != nil {
		t.Fatalf("delete --json: %v", err)
	}
	if !strings.Contains(out, `"inbound":[]`) {
		t.Errorf("expected an empty array, got:\n%s", out)
	}
}

func TestMoveCmdJSON(t *testing.T) {
	cfg := bundleAt(t)
	targetPath, targetID, err := bundle.CreateConcept(cfg, bundle.Spec{Title: "Target"})
	if err != nil {
		t.Fatal(err)
	}
	targetRel, err := filepath.Rel(cfg.RootPath, targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bundle.CreateConcept(cfg, bundle.Spec{
		Title: "Linker",
		Body:  "See [T](/" + filepath.ToSlash(targetRel) + ").",
	}); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, newMoveCmd(), targetID, "archive/", "--json")
	if err != nil {
		t.Fatalf("move --json: %v", err)
	}
	var got struct {
		From     string   `json:"from"`
		To       string   `json:"to"`
		FromRel  string   `json:"from_rel"`
		ToRel    string   `json:"to_rel"`
		Rewrote  []string `json:"rewrote"`
		SelfLink bool     `json:"self_link"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	// `to` must be absolute: the plugin :edit's it directly.
	if !filepath.IsAbs(got.To) {
		t.Errorf("to = %q, want an absolute path", got.To)
	}
	if _, err := os.Stat(got.To); err != nil {
		t.Errorf("to = %q does not exist: %v", got.To, err)
	}
	if got.From != targetPath {
		t.Errorf("from = %q, want %q", got.From, targetPath)
	}
	if got.ToRel != "archive/"+filepath.Base(targetPath) {
		t.Errorf("to_rel = %q", got.ToRel)
	}
	if len(got.Rewrote) != 1 {
		t.Errorf("rewrote = %v, want the one linker", got.Rewrote)
	}
	if got.SelfLink {
		t.Error("self_link should be false")
	}
}

func TestMoveCmdJSONRewroteIsNeverNull(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	out, err := runCmd(t, newMoveCmd(), id, "archive/", "--json")
	if err != nil {
		t.Fatalf("move --json: %v", err)
	}
	if !strings.Contains(out, `"rewrote":[]`) {
		t.Errorf("expected an empty array, got:\n%s", out)
	}
}
