package celerisserver

import "time"

// Claims are what one set of credentials grants. Your server decides them
// for a user it has authenticated; nothing from the client widens them.
//
// The zero value is invalid: Channels and Permissions must be chosen
// explicitly, so unrestricted access is never granted by omission (AUTH-02).
type Claims struct {
	// Channels the credentials may connect to.
	Channels ChannelScope

	// Permissions on the segments of those channels.
	Permissions SegmentPermissions

	// Reference identifies the token, such as a user id, in presence and
	// deliveries. Empty leaves it out.
	Reference string

	// Replay is the history replayed on each segment join. The zero value
	// replays nothing.
	Replay Replay

	// AllowEcho lets a connection receive its own publishes.
	AllowEcho bool
} // end struct Claims

// ChannelScope is the set of channels credentials may connect to. Make one
// with [AllChannels] or [RestrictedChannels].
type ChannelScope struct {
	kind       scopeKind
	references []string
} // end struct ChannelScope

// SegmentPermissions grant read and write access to segments. Make one with
// [AllSegments] or [RestrictedSegments].
type SegmentPermissions struct {
	kind     scopeKind
	read     bool
	write    bool
	segments []SegmentClaim
} // end struct SegmentPermissions

// SegmentClaim grants access to one segment.
type SegmentClaim struct {
	SegmentID string
	Read      bool
	Write     bool
} // end struct SegmentClaim

// Replay is the history replayed when a connection joins a segment. Make one
// with [ReplayBacklog] or [ReplayLookback]; the zero value replays nothing.
type Replay struct {
	kind     replayKind
	lookback time.Duration
} // end struct Replay

type scopeKind int

const (
	unset scopeKind = iota
	all
	restricted
)

type replayKind int

const (
	noReplay replayKind = iota
	backlogReplay
	lookbackReplay
)

// AllChannels grants every channel of the application.
func AllChannels() ChannelScope {
	return ChannelScope{kind: all}
} // end function AllChannels

// RestrictedChannels grants only the listed channels: at least one, each
// listed once.
func RestrictedChannels(references ...string) ChannelScope {
	return ChannelScope{kind: restricted, references: append([]string(nil), references...)}
} // end function RestrictedChannels

// AllSegments grants the same access to every segment.
func AllSegments(read, write bool) SegmentPermissions {
	return SegmentPermissions{kind: all, read: read, write: write}
} // end function AllSegments

// RestrictedSegments grants access to the listed segments only, each listed
// once. With none listed, the credentials can connect but neither read nor
// write.
func RestrictedSegments(segments ...SegmentClaim) SegmentPermissions {
	return SegmentPermissions{kind: restricted, segments: append([]SegmentClaim(nil), segments...)}
} // end function RestrictedSegments

// ReplayBacklog replays everything the server still retains for a segment.
func ReplayBacklog() Replay {
	return Replay{kind: backlogReplay}
} // end function ReplayBacklog

// ReplayLookback replays what a segment received within lookback before the
// join: whole milliseconds, from 0 to 4294967295. Zero replays nothing older
// than the join.
func ReplayLookback(lookback time.Duration) Replay {
	return Replay{kind: lookbackReplay, lookback: lookback}
} // end function ReplayLookback
