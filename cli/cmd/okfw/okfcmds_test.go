package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"okfw/cli/internal/bundle"
	"okfw/cli/internal/config"
)

// bundleAt creates a wiki in a temp dir, makes it the working directory (so the
// commands' own config.Discover("") finds it), and returns the config.
func bundleAt(t *testing.T) *config.Config {
	t.Helper()
	project := t.TempDir()
	if err := runInit(project); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	t.Chdir(project)
	cfg, err := config.Discover(project)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return cfg
}

// runCmd executes one of the family writers with the given args.
func runCmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// readConcept returns the single concept in the bundle root.
func readConcept(t *testing.T, cfg *config.Config) (string, *config.Concept) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(cfg.RootPath, "*--*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected exactly 1 concept, got %v (%v)", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	z, err := config.ParseAndValidate(content)
	if err != nil {
		t.Fatalf("concept does not validate: %v\n%s", err, content)
	}
	return string(content), z
}

func seedConcept(t *testing.T, cfg *config.Config) string {
	t.Helper()
	_, id, err := bundle.CreateConcept(cfg, bundle.Spec{Title: "Subject"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// --- lifecycle ---

func TestSetLifecycleCmd(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newSetLifecycleCmd(), id, "deprecated"); err != nil {
		t.Fatalf("set-lifecycle: %v", err)
	}
	raw, z := readConcept(t, cfg)
	if z.LifecycleStatus() != "deprecated" {
		t.Errorf("status = %q\n%s", z.LifecycleStatus(), raw)
	}
}

func TestSetLifecycleCmdRejectsBadValues(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	// The removed todo vocabulary must not sneak back in through this door.
	for _, bad := range []string{"open", "closed", "in_progress", "nonsense"} {
		if _, err := runCmd(t, newSetLifecycleCmd(), id, bad); err == nil {
			t.Errorf("set-lifecycle %q should fail", bad)
		}
	}
	_, z := readConcept(t, cfg)
	if z.Status != "" {
		t.Errorf("a rejected value must not be written, got %q", z.Status)
	}
}

func TestSetStaleAfterCmdAndClear(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newSetStaleAfterCmd(), id, "2026-12-31"); err != nil {
		t.Fatalf("set-stale-after: %v", err)
	}
	_, z := readConcept(t, cfg)
	// A bare date is accepted at the CLI and written in the OKF form.
	if z.StaleAfter != "2026-12-31T00:00:00Z" {
		t.Fatalf("stale_after = %q", z.StaleAfter)
	}
	if _, err := runCmd(t, newSetStaleAfterCmd(), id, "2027-01-01T09:00:00+05:30"); err != nil {
		t.Fatalf("set-stale-after with an offset: %v", err)
	}
	if _, z = readConcept(t, cfg); z.StaleAfter != "2027-01-01T09:00:00+05:30" {
		t.Fatalf("a datetime must be kept as written, got %q", z.StaleAfter)
	}

	if _, err := runCmd(t, newSetStaleAfterCmd(), id, "--clear"); err != nil {
		t.Fatalf("--clear: %v", err)
	}
	raw, z := readConcept(t, cfg)
	if z.StaleAfter != "" || strings.Contains(raw, "stale_after") {
		t.Errorf("--clear should remove the field:\n%s", raw)
	}
}

func TestSetStaleAfterCmdRejectsBadDate(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	for _, bad := range []string{"31-12-2026", "2026-13-01", "tomorrow", "2026-12-31T00:00:00"} {
		if _, err := runCmd(t, newSetStaleAfterCmd(), id, bad); err == nil {
			t.Errorf("set-stale-after %q should fail", bad)
		}
	}
}

// --- trust ---

func TestVerifyCmdAppendsAndRaisesTrustTier(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	_, z := readConcept(t, cfg)
	if z.TrustTier() != config.TrustUnverified {
		t.Fatalf("a fresh concept should be unverified, got %q", z.TrustTier())
	}

	// A machine actor yields machine-confirmed.
	if _, err := runCmd(t, newVerifyCmd(), id, "--by", "process:nightly", "--at", "2026-07-20T10:00:00Z"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	_, z = readConcept(t, cfg)
	if z.TrustTier() != config.TrustMachineConfirmed {
		t.Errorf("tier = %q, want %q", z.TrustTier(), config.TrustMachineConfirmed)
	}

	// A human actor anywhere in the list wins.
	if _, err := runCmd(t, newVerifyCmd(), id, "--by", "human:ryan", "--at", "2026-07-21T10:00:00Z"); err != nil {
		t.Fatalf("verify: %v", err)
	}
	raw, z := readConcept(t, cfg)
	if len(z.Verified) != 2 {
		t.Fatalf("expected 2 verification entries, got %d:\n%s", len(z.Verified), raw)
	}
	if z.TrustTier() != config.TrustHumanReviewed {
		t.Errorf("tier = %q, want %q", z.TrustTier(), config.TrustHumanReviewed)
	}
	// "How recently" is the latest at.
	if got := z.LastVerifiedAt().Format("2006-01-02"); got != "2026-07-21" {
		t.Errorf("LastVerifiedAt = %s", got)
	}
}

// A file already using OKF's single-entry shorthand must become a list rather
// than gaining a second verified key.
func TestVerifyCmdUpgradesBareMapping(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	path, err := bundle.ResolveConceptPath(cfg, id)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	withBare := strings.Replace(string(content), "---\n\n# ",
		"verified:\n  by: \"process:first\"\n  at: \"2026-07-01T10:00:00Z\"\n---\n\n# ", 1)
	if err := os.WriteFile(path, []byte(withBare), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := runCmd(t, newVerifyCmd(), id, "--by", "human:ryan"); err != nil {
		t.Fatalf("verify over a bare mapping: %v", err)
	}
	raw, z := readConcept(t, cfg)
	if strings.Count(raw, "verified:") != 1 {
		t.Errorf("verified key duplicated:\n%s", raw)
	}
	if len(z.Verified) != 2 {
		t.Errorf("expected both entries, got %d:\n%s", len(z.Verified), raw)
	}
}

func TestVerifyCmdDefaultsToAHumanActor(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newVerifyCmd(), id); err != nil {
		t.Fatalf("verify: %v", err)
	}
	_, z := readConcept(t, cfg)
	// Running the command by hand is a human confirmation, so the default actor
	// must land in the human tier.
	if z.TrustTier() != config.TrustHumanReviewed {
		t.Errorf("tier = %q, want %q", z.TrustTier(), config.TrustHumanReviewed)
	}
	if !strings.HasPrefix(z.Verified[0].By, config.HumanActorPrefix) {
		t.Errorf("default actor = %q, want a human: prefix", z.Verified[0].By)
	}
}

// TestActorWritersRefuseAMalformedActor: humna:alice would have been written
// and read back as machine-confirmed. The writers refuse it even on an
// already-invalid concept, where the write guard lets any edit through.
func TestActorWritersRefuseAMalformedActor(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	matches, _ := filepath.Glob(filepath.Join(cfg.RootPath, "*--*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 concept, got %v", matches)
	}
	pristine, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	broken := append(append([]byte{}, pristine...), []byte("\nUncited.[^nowhere]\n")...)

	for _, content := range [][]byte{pristine, broken} {
		if err := os.WriteFile(matches[0], content, 0644); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name string
			cmd  *cobra.Command
		}{{"verify", newVerifyCmd()}, {"set-generated", newSetGeneratedCmd()}} {
			if _, err := runCmd(t, tc.cmd, id, "--by", "humna:alice"); err == nil || !strings.Contains(err.Error(), "OKF section 7") {
				t.Errorf("%s --by humna:alice: err = %v, want a refusal", tc.name, err)
			}
		}
		if got, _ := os.ReadFile(matches[0]); !bytes.Equal(got, content) {
			t.Errorf("a refused write changed the file:\n%s", got)
		}
	}
}

func TestSetGeneratedCmd(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newSetGeneratedCmd(), id, "--by", "process:finance-nightly"); err != nil {
		t.Fatalf("set-generated: %v", err)
	}
	_, z := readConcept(t, cfg)
	if z.Generated == nil || z.Generated.By != "process:finance-nightly" {
		t.Fatalf("generated = %+v", z.Generated)
	}
	// generated.at must equal timestamp, or ParseAndValidate would have failed
	// in readConcept — assert it explicitly so the coupling is visible.
	if z.Generated.At == "" {
		t.Error("generated.at should be stamped alongside by")
	}
}

// --- provenance ---

func TestAddSourceCmd(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newAddSourceCmd(), id,
		"--resource", "https://example.com/schema",
		"--id", "px", "--title", "Price schema", "--author", "team:data",
		"--usage-count", "412", "--last-modified", "2026-05-01",
		"--usage-window", "2026-01-01..2026-06-30",
	); err != nil {
		t.Fatalf("add-source: %v", err)
	}
	raw, z := readConcept(t, cfg)
	if len(z.Sources) != 1 {
		t.Fatalf("expected 1 source, got %d:\n%s", len(z.Sources), raw)
	}
	s := z.Sources[0]
	if s.Resource != "https://example.com/schema" || s.ID != "px" || s.Author != "team:data" {
		t.Errorf("source = %+v", s)
	}
	if s.UsageCount == nil || *s.UsageCount != 412 {
		t.Errorf("usage_count = %v", s.UsageCount)
	}
	if s.LastModified != "2026-05-01T00:00:00Z" {
		t.Errorf("last_modified = %q, want the bare date normalized", s.LastModified)
	}
	if s.UsageWindow == nil || s.UsageWindow.From != "2026-01-01T00:00:00Z" || s.UsageWindow.To != "2026-06-30T00:00:00Z" {
		t.Errorf("usage_window = %+v", s.UsageWindow)
	}

	// A second source appends rather than replacing.
	if _, err := runCmd(t, newAddSourceCmd(), id, "--resource", "all queries in project X"); err != nil {
		t.Fatalf("second add-source: %v", err)
	}
	_, z = readConcept(t, cfg)
	if len(z.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(z.Sources))
	}
	// A scope descriptor is a legal resource, not a path.
	if z.Sources[1].Resource != "all queries in project X" {
		t.Errorf("second source = %+v", z.Sources[1])
	}
}

// TestAddSourceRefusesADuplicatedID: a second entry with an id already in
// sources makes every [^id] citation ambiguous. The mutator refuses it itself,
// because the write guard lets any edit through to an already-invalid concept.
func TestAddSourceRefusesADuplicatedID(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	if _, err := runCmd(t, newAddSourceCmd(), id, "--resource", "https://a.example", "--id", "ga4"); err != nil {
		t.Fatalf("add-source: %v", err)
	}
	for _, dup := range []string{"ga4", "GA4"} {
		if _, err := runCmd(t, newAddSourceCmd(), id, "--resource", "https://b.example", "--id", dup); err == nil ||
			!strings.Contains(err.Error(), "already has an entry with id") {
			t.Errorf("--id %s: err = %v, want a refusal", dup, err)
		}
	}

	// Break the concept, so the write guard would let anything through.
	matches, _ := filepath.Glob(filepath.Join(cfg.RootPath, "*--*.md"))
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 concept, got %v", matches)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("\nUncited.[^nowhere]\n")...)
	if _, err := config.ParseAndValidate(content); err == nil {
		t.Fatal("setup: the concept should now be invalid")
	}
	if err := os.WriteFile(matches[0], content, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, newAddSourceCmd(), id, "--resource", "https://b.example", "--id", "ga4"); err == nil {
		t.Error("a duplicated id was appended to an already-invalid concept")
	}
	// The repair the error message suggests still goes through.
	if _, err := runCmd(t, newAddSourceCmd(), id, "--resource", "https://n.example", "--id", "nowhere"); err != nil {
		t.Fatalf("add-source --id nowhere: %v", err)
	}
	if _, z := readConcept(t, cfg); len(z.Sources) != 2 {
		t.Errorf("sources = %+v, want ga4 and nowhere", z.Sources)
	}
}

func TestAddSourceCmdRequiresResource(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	if _, err := runCmd(t, newAddSourceCmd(), id, "--id", "orphan"); err == nil {
		t.Error("add-source without --resource should fail")
	}
	if _, err := runCmd(t, newAddSourceCmd(), id,
		"--resource", "x", "--usage-window", "2026-01-01"); err == nil {
		t.Error("a malformed --usage-window should fail")
	}
}

func TestSetResourceCmd(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)

	if _, err := runCmd(t, newSetResourceCmd(), id, "bigquery://proj.ds.orders"); err != nil {
		t.Fatalf("set-resource: %v", err)
	}
	_, z := readConcept(t, cfg)
	if z.Resource != "bigquery://proj.ds.orders" {
		t.Errorf("resource = %q", z.Resource)
	}
}

// --- shared behaviour ---

func TestFamilyWritersRejectAnUnknownRef(t *testing.T) {
	bundleAt(t)
	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
		args []string
	}{
		{"set-lifecycle", newSetLifecycleCmd(), []string{"019680f0-7e00-7abc-8def-000000000000", "draft"}},
		{"verify", newVerifyCmd(), []string{"019680f0-7e00-7abc-8def-000000000000"}},
		{"set-resource", newSetResourceCmd(), []string{"019680f0-7e00-7abc-8def-000000000000", "x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := runCmd(t, tc.cmd, tc.args...); err == nil {
				t.Error("expected a not-found error")
			}
		})
	}
}

// Each writer bumps the OKF timestamp, since each is a meaningful change.
func TestFamilyWritersBumpTimestamp(t *testing.T) {
	cfg := bundleAt(t)
	id := seedConcept(t, cfg)
	_, before := readConcept(t, cfg)

	if _, err := runCmd(t, newSetLifecycleCmd(), id, "draft"); err != nil {
		t.Fatal(err)
	}
	_, after := readConcept(t, cfg)
	if !after.Timestamp.After(before.Timestamp) && !after.Timestamp.Equal(before.Timestamp) {
		t.Errorf("timestamp went backwards: %v -> %v", before.Timestamp, after.Timestamp)
	}
	if after.Timestamp.Before(before.Created) {
		t.Error("timestamp predates created")
	}
}

func TestOkfFamilyCommandsAreAllRegistered(t *testing.T) {
	want := map[string]bool{
		"set-lifecycle": false, "set-stale-after": false, "verify": false,
		"add-source": false, "set-resource": false, "set-generated": false,
	}
	for _, c := range okfFamilyCommands() {
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
			t.Errorf("command %q is not registered", name)
		}
	}
}
