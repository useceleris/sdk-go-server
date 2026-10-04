package celerisserver

import "time"

// Every fixed value the package uses, in one place. None of these is part of
// the public surface.

const (
	// The latest timestamp the server accepts: the last millisecond of the
	// year 9999.
	maximumTimestamp = 253402300799999

	// The server's largest replay lookback: an unsigned 32-bit millisecond
	// count.
	maximumReplayLookback = 4_294_967_295 * time.Millisecond

	// The longest channel reference the server accepts.
	maximumChannelReferenceLength = 255
)
