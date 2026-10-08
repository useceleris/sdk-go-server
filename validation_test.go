package celerisserver

import (
	"strings"
	"testing"
	"time"
)

func TestClaimsAreValidated(t *testing.T) {
	permissions := AllSegments(true, true)
	cases := []struct {
		claims  Claims
		message string
	}{
		{Claims{}, "Invalid claims. Channels: Required. Permissions: Required."},
		{Claims{Channels: AllChannels()}, "Invalid claims. Permissions: Required."},
		{Claims{Channels: RestrictedChannels(), Permissions: permissions}, "Invalid claims. Channels.References: Must list at least one channel reference."},
		{Claims{Channels: RestrictedChannels("a", "a"), Permissions: permissions}, "Invalid claims. Channels.References: Must not repeat a channel reference."},
		{Claims{Channels: RestrictedChannels("ok", ""), Permissions: permissions}, "Invalid claims. Channels.References[1]: Must not be empty."},
		{Claims{Channels: RestrictedChannels("x:y"), Permissions: permissions}, "Invalid claims. Channels.References[0]: Must contain only ASCII letters, digits, hyphens (-) or underscores (_)."},
		{Claims{Channels: RestrictedChannels("é"), Permissions: permissions}, "Invalid claims. Channels.References[0]: Must contain only ASCII letters, digits, hyphens (-) or underscores (_)."},
		{Claims{Channels: RestrictedChannels(strings.Repeat("x", 256)), Permissions: permissions}, "Invalid claims. Channels.References[0]: Must be at most 255 characters."},
		{Claims{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{SegmentID: "a\n"})}, "Invalid claims. Permissions.Segments[0].SegmentID: Must not contain CR or LF."},
		{Claims{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{})}, "Invalid claims. Permissions.Segments[0].SegmentID: Must not be empty."},
		{Claims{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{SegmentID: "s\xed\xa0\x80"})}, "Invalid claims. Permissions.Segments[0].SegmentID: Must not contain invalid UTF-8."},
		{Claims{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{SegmentID: "s", Read: true}, SegmentClaim{SegmentID: "s", Write: true})}, "Invalid claims. Permissions.Segments: Must not repeat a segment ID."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Reference: "user:123"}, "Invalid claims. Reference: Must not contain a colon, CR or LF."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Reference: "user\r"}, "Invalid claims. Reference: Must not contain a colon, CR or LF."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Reference: "user\xff"}, "Invalid claims. Reference: Must not contain invalid UTF-8."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Replay: ReplayLookback(-time.Millisecond)}, "Invalid claims. Replay: Lookback must be between 0 and 4294967295 milliseconds."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Replay: ReplayLookback(4294967296 * time.Millisecond)}, "Invalid claims. Replay: Lookback must be between 0 and 4294967295 milliseconds."},
		{Claims{Channels: AllChannels(), Permissions: permissions, Replay: ReplayLookback(1500 * time.Microsecond)}, "Invalid claims. Replay: Lookback must be whole milliseconds."},
	}

	for _, test := range cases {
		assertConfiguration(t, validateClaims(test.claims), test.message)
	}
} // end function TestClaimsAreValidated

func TestValidClaimsAcrossTheirRange(t *testing.T) {
	valid := []Claims{
		{Channels: RestrictedChannels("aZ09-_", strings.Repeat("x", 255), "x"), Permissions: RestrictedSegments()},
		{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{SegmentID: "s:with:colons 😀 \x00 \t"})},
		{Channels: AllChannels(), Permissions: AllSegments(false, false), Reference: "身分 😀", Replay: ReplayLookback(0)},
		{Channels: AllChannels(), Permissions: AllSegments(true, false), Replay: ReplayLookback(4294967295 * time.Millisecond)},
		{Channels: AllChannels(), Permissions: AllSegments(false, true), Replay: ReplayBacklog(), AllowEcho: true},
	}

	for index, claims := range valid {
		if err := validateClaims(claims); err != nil {
			t.Errorf("claims %d refused: %v", index, err)
		}
	}
} // end function TestValidClaimsAcrossTheirRange

func TestTokenPayloadLayout(t *testing.T) {
	cases := []struct {
		claims Claims
		json   string
	}{
		{
			Claims{Channels: RestrictedChannels("room-1"), Permissions: RestrictedSegments()},
			`{"timestamp":5,"channel_references":["room-1"],"token_permission":[],"replay":false,"allow_echo":false}`,
		},
		{
			Claims{Channels: AllChannels(), Permissions: AllSegments(true, false), Reference: "<u&>", Replay: ReplayBacklog(), AllowEcho: true},
			`{"timestamp":5,"reference":"<u&>","channel_references":null,"token_permission":{"read":true,"write":false},"replay":true,"allow_echo":true}`,
		},
		{
			Claims{Channels: AllChannels(), Permissions: RestrictedSegments(SegmentClaim{SegmentID: "a", Write: true}), Replay: ReplayLookback(0)},
			`{"timestamp":5,"channel_references":null,"token_permission":[{"segment_id":"a","read":false,"write":true}],"replay":0,"allow_echo":false}`,
		},
	}

	for _, test := range cases {
		if got := string(appendTokenPayload(nil, 5, test.claims)); got != test.json {
			t.Errorf("payload %s, want %s", got, test.json)
		}
	}
} // end function TestTokenPayloadLayout

func TestChannelReferenceCharacterClassEdges(t *testing.T) {
	if rule := channelReferenceRule("azAZ09-_"); rule != "" {
		t.Fatalf("edge characters refused: %s", rule)
	}

	for _, character := range []string{"`", "{", "@", "[", "/", ":", " ", "."} {
		if channelReferenceRule("a"+character+"b") == "" {
			t.Errorf("%q accepted", character)
		}
	}
} // end function TestChannelReferenceCharacterClassEdges
