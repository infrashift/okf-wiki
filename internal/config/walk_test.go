package config

import "testing"

func TestIsReservedBase(t *testing.T) {
	if !IsReservedBase("index.md") || !IsReservedBase("log.md") {
		t.Error("index.md/log.md should be reserved")
	}
	if IsReservedBase("note--abcd.md") {
		t.Error("regular concept should not be reserved")
	}
}

func TestIsConceptFile(t *testing.T) {
	for name, want := range map[string]bool{
		"note--abcd.md": true,
		".dot.md":       true, // only directories are hidden from a walk
		"index.md":      false,
		"log.md":        false,
		"notes.txt":     false,
		"README.MD":     false,
	} {
		if got := IsConceptFile(name); got != want {
			t.Errorf("IsConceptFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestSkipBundleDir(t *testing.T) {
	for rel, want := range map[string]bool{
		".":                false, // the root, though it is the dot-directory .okfw
		"":                 false,
		"notes":            false,
		"ephemeral":        true,
		"ephemeral/":       true,
		"foo/ephemeral":    false, // only the root's ephemeral/ is okfw's
		".git":             true,
		"notes/.hidden":    true,
		"notes/.hidden/x":  false, // a walk never reaches it: .hidden is skipped
		"decisions/v1.2/x": false,
	} {
		if got := SkipBundleDir(rel); got != want {
			t.Errorf("SkipBundleDir(%q) = %v, want %v", rel, got, want)
		}
	}
}
