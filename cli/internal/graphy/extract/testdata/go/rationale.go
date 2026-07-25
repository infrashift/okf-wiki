package rationale

import "errors"

// ErrClosed is returned after Close.
// WHY: callers distinguish a closed store from a missing key, so this must
// stay a sentinel rather than a fresh error value.
var ErrClosed = errors.New("closed")

// Store holds records.
type Store struct {
	closed bool
}

// Put writes a record.
//
// DECISION: writes go through a single transaction so a partial batch can
// never be observed.
func (s *Store) Put(key string, value []byte) error {
	if s.closed {
		return ErrClosed
	}
	// HACK: the upstream driver panics on a nil value, so normalize here.
	if value == nil {
		value = []byte{}
	}
	msg := "TODO: this string is data, not a comment"
	_ = msg
	return nil
}

/*
RATIONALE: the block-comment form is supported too, and continuation
lines join the body.
*/
func helper() {}
