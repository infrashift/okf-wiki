package config

import "testing"

func TestVersionPrefersTheStampedValue(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	version = "v2026.10.0"
	if got := Version(); got != "v2026.10.0" {
		t.Fatalf("Version() = %q, want the stamped v2026.10.0", got)
	}
}

// A test binary carries no main-module version, so this exercises the path a
// `go build` without make takes when Go recorded nothing either.
func TestVersionIsNeverEmptyUnstamped(t *testing.T) {
	old := version
	t.Cleanup(func() { version = old })

	version = ""
	if got := Version(); got == "" {
		t.Fatal("Version() is empty for an unstamped build")
	}
}
