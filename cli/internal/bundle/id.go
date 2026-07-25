package bundle

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NewV7WithTime constructs a UUIDv7 with a specific timestamp instead of time.Now().
func NewV7WithTime(t time.Time) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	ms := t.UnixMilli()
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0F) | 0x70 // version 7
	b[8] = (b[8] & 0x3F) | 0x80 // variant 10
	return uuid.UUID(b).String()
}

// TimeFromID extracts creation time from either ID format:
//   - Legacy: "YYYYMMDDHHmmss-UUID" → parse 14-char prefix
//   - UUIDv7: parse first 48 bits as Unix milliseconds
func TimeFromID(id string) (time.Time, error) {
	if len(id) > 36 && id[14] == '-' {
		t, err := time.Parse("20060102150405", id[:14])
		if err == nil {
			return t, nil
		}
	}
	u, err := uuid.Parse(id)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid ID: %s", id)
	}
	ms := int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
	return time.UnixMilli(ms), nil
}
