package bundle

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewV7WithTime(t *testing.T) {
	want := time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)
	id := NewV7WithTime(want)

	u, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("NewV7WithTime produced an unparseable uuid %q: %v", id, err)
	}
	if u.Version() != 7 {
		t.Errorf("version = %d, want 7", u.Version())
	}
	if u.Variant() != uuid.RFC4122 {
		t.Errorf("variant = %v, want RFC4122", u.Variant())
	}
	got, err := TimeFromID(id)
	if err != nil {
		t.Fatalf("TimeFromID: %v", err)
	}
	if !got.UTC().Equal(want) {
		t.Errorf("round-trip time = %v, want %v", got.UTC(), want)
	}
}

// Two ids minted for the same instant must still differ — the timestamp fills
// only the first 48 bits, the rest is random.
func TestNewV7WithTimeIsUniquePerCall(t *testing.T) {
	at := time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewV7WithTime(at)
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// UUIDv7 ids sort chronologically as strings, which is what makes the 12-hex
// short id in filenames sort by creation time.
func TestNewV7WithTimeSortsChronologically(t *testing.T) {
	early := NewV7WithTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	late := NewV7WithTime(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if !(early < late) {
		t.Errorf("expected %q < %q", early, late)
	}
}

func TestTimeFromIDLegacyFormat(t *testing.T) {
	got, err := TimeFromID("20260213104500-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if err != nil {
		t.Fatalf("TimeFromID: %v", err)
	}
	want := time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)
	if !got.UTC().Equal(want) {
		t.Errorf("got %v, want %v", got.UTC(), want)
	}
}

func TestTimeFromIDUUIDv7(t *testing.T) {
	// 019680f07e00 ms since epoch, the timestamp okfw encodes in the filename.
	got, err := TimeFromID("019680f0-7e00-7abc-8def-1234567890ab")
	if err != nil {
		t.Fatalf("TimeFromID: %v", err)
	}
	if got.IsZero() {
		t.Error("expected a real time")
	}
	if got.Year() < 2020 || got.Year() > 2100 {
		t.Errorf("implausible year %d from a v7 id", got.Year())
	}
}

func TestTimeFromIDInvalid(t *testing.T) {
	for _, bad := range []string{"", "12345", "not-a-uuid", "zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"} {
		if _, err := TimeFromID(bad); err == nil {
			t.Errorf("TimeFromID(%q) should fail", bad)
		}
	}
}

// A legacy id whose 14-char prefix is not a real date must fall through to
// uuid parsing rather than being reported as that bogus date.
func TestTimeFromIDRejectsBogusLegacyPrefix(t *testing.T) {
	if _, err := TimeFromID("99999999999999-aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"); err == nil {
		t.Error("expected an error for an unparseable legacy prefix and invalid uuid")
	}
}
