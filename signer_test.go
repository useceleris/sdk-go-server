package celerisserver

import (
	"errors"
	"strings"
	"testing"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
)

func scopedClaims() Claims {
	return Claims{
		Channels:    RestrictedChannels("room-1"),
		Permissions: RestrictedSegments(SegmentClaim{SegmentID: "messages", Read: true}),
	}
} // end function scopedClaims

func fixedSigner(t *testing.T, clock func() time.Time) *Signer {
	t.Helper()

	signer, err := NewSigner(SignerOptions{ClientID: "synthetic-client", SigningSecret: "synthetic-secret", Clock: clock})

	if err != nil {
		t.Fatal(err)
	}

	return signer
} // end function fixedSigner

func assertConfiguration(t *testing.T, err error, message string) {
	t.Helper()

	sdkError, ok := errors.AsType[*celeris.Error](err)

	if !ok || sdkError.Code != celeris.ErrConfiguration {
		t.Fatalf("got %v, want a configuration error", err)
	}

	if message != "" && sdkError.Message != message {
		t.Fatalf("message %q, want %q", sdkError.Message, message)
	}

	if errors.Unwrap(err) != nil {
		t.Fatalf("error wraps a cause: %v", err)
	}
} // end function assertConfiguration

func TestSigningUsesAFreshTimestampEachTime(t *testing.T) {
	timestamp := int64(123456789)
	signer := fixedSigner(t, func() time.Time {
		timestamp++

		return time.UnixMilli(timestamp)
	})

	first, err := signer.Sign(scopedClaims())

	if err != nil {
		t.Fatal(err)
	}

	second, _ := signer.Sign(scopedClaims())

	if first == second {
		t.Fatal("two signings produced the same credentials")
	}

	echo := scopedClaims()
	echo.AllowEcho = true

	if third, _ := signer.Sign(echo); third == second {
		t.Fatal("changed claims signed the same")
	}
} // end function TestSigningUsesAFreshTimestampEachTime

func TestSignerOptionsAreValidated(t *testing.T) {
	cases := []struct {
		options SignerOptions
		message string
	}{
		{SignerOptions{}, "Invalid signer options. ClientID: Must not be empty. SigningSecret: Must not be empty."},
		{SignerOptions{ClientID: "a:b", SigningSecret: "s"}, "Invalid signer options. ClientID: Must not contain a colon, CR or LF."},
		{SignerOptions{ClientID: "a\r", SigningSecret: "s"}, "Invalid signer options. ClientID: Must not contain a colon, CR or LF."},
		{SignerOptions{ClientID: "a\n", SigningSecret: "s"}, "Invalid signer options. ClientID: Must not contain a colon, CR or LF."},
		{SignerOptions{ClientID: "client\xed\xa0\x80", SigningSecret: "s"}, "Invalid signer options. ClientID: Must not contain invalid UTF-8."},
		{SignerOptions{ClientID: "c", SigningSecret: "synthetic-secret\xff"}, "Invalid signer options. SigningSecret: Must not contain invalid UTF-8."},
	}

	for _, test := range cases {
		_, err := NewSigner(test.options)
		assertConfiguration(t, err, test.message)

		if strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatalf("error repeats the secret: %v", err)
		}
	}
} // end function TestSignerOptionsAreValidated

func TestInvalidClaimsAreRefusedBeforeTheClockIsRead(t *testing.T) {
	read := false
	signer := fixedSigner(t, func() time.Time {
		read = true

		return time.UnixMilli(1)
	})

	_, err := signer.Sign(Claims{Channels: RestrictedChannels()})
	assertConfiguration(t, err, "Invalid claims. Channels.References: Must list at least one channel reference. Permissions: Required.")

	if read {
		t.Fatal("the clock was read for invalid claims")
	}
} // end function TestInvalidClaimsAreRefusedBeforeTheClockIsRead

func TestClockOutsideTheServersRangeIsRefused(t *testing.T) {
	// Far beyond the range UnixMilli is defined for, which must not wrap into
	// the valid window.
	overflow := time.Unix(18446744073709552, 0)

	for _, moment := range []time.Time{time.UnixMilli(0), time.UnixMilli(-5), time.UnixMilli(253402300800000), overflow, time.Unix(-18446744073709552, 0)} {
		_, err := fixedSigner(t, func() time.Time { return moment }).Sign(scopedClaims())
		assertConfiguration(t, err, "Invalid timestamp from Clock. Must be between 1 and 253402300799999 milliseconds.")
	}

	if _, err := fixedSigner(t, func() time.Time { return time.UnixMilli(253402300799999) }).Sign(scopedClaims()); err != nil {
		t.Fatalf("latest timestamp refused: %v", err)
	}
} // end function TestClockOutsideTheServersRangeIsRefused

func TestSignerDefaultsToTheWallClock(t *testing.T) {
	before := time.Now().UnixMilli()
	credentials, err := fixedSigner(t, nil).Sign(scopedClaims())

	if err != nil || credentials.Payload == "" {
		t.Fatalf("got %v", err)
	}

	if after := time.Now().UnixMilli(); after < before {
		t.Fatal("clock went backwards")
	}
} // end function TestSignerDefaultsToTheWallClock

func TestCallerChangesAfterConstructionOrSigningChangeNothing(t *testing.T) {
	references := []string{"room-1"}
	segments := []SegmentClaim{{SegmentID: "messages", Read: true}}
	claims := Claims{Channels: RestrictedChannels(references...), Permissions: RestrictedSegments(segments...)}
	signer := fixedSigner(t, func() time.Time { return time.UnixMilli(123456789) })
	before, _ := signer.Sign(claims)

	references[0] = "other"
	segments[0].Write = true

	if after, _ := signer.Sign(claims); after != before {
		t.Fatal("claims shared the caller's slices")
	}

	options := SignerOptions{ClientID: "synthetic-client", SigningSecret: "synthetic-secret", Clock: func() time.Time { return time.UnixMilli(123456789) }}
	copied, _ := NewSigner(options)
	options.SigningSecret = "changed"

	if after, _ := copied.Sign(claims); after != before {
		t.Fatal("the signer shared the caller's options")
	}
} // end function TestCallerChangesAfterConstructionOrSigningChangeNothing
