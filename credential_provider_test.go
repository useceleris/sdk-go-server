package celerisserver

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
)

// countingSigner signs with a clock that counts its reads, so a test can tell
// whether signing happened.
func countingSigner(t *testing.T) (*Signer, *atomic.Int64) {
	t.Helper()

	var reads atomic.Int64

	return fixedSigner(t, func() time.Time { return time.UnixMilli(1_000 + reads.Add(1)) }), &reads
}

func decodedPayload(t *testing.T, credentials celeris.Credentials) string {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(credentials.Payload)

	if err != nil {
		t.Fatal(err)
	}

	return string(decoded)
}

func TestProviderCallsClaimsAndSignsAfreshEachAttempt(t *testing.T) {
	signer, reads := countingSigner(t)
	var calls atomic.Int64
	provider, err := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		calls.Add(1)

		return scopedClaims(), nil
	})

	if err != nil {
		t.Fatal(err)
	}

	first, err := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "room-1"})

	if err != nil {
		t.Fatal(err)
	}

	second, _ := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "room-1"})

	if first == second || calls.Load() != 2 || reads.Load() != 2 {
		t.Fatalf("calls %d, reads %d", calls.Load(), reads.Load())
	}
}

func TestClaimsDecideReplayFromTheReconnectRequest(t *testing.T) {
	signer, _ := countingSigner(t)
	provider, _ := NewCredentialProvider(signer, func(_ context.Context, request celeris.CredentialRequest) (Claims, error) {
		claims := scopedClaims()

		if request.Reconnect {
			claims.Replay = ReplayLookback(min(request.ReplayLookback, 30*time.Second))
		}

		return claims, nil
	})

	initial, _ := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "room-1"})
	reconnect, _ := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "room-1", Reconnect: true, ReplayLookback: 6500 * time.Millisecond})
	capped, _ := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "room-1", Reconnect: true, ReplayLookback: time.Hour})

	for credentials, replay := range map[celeris.Credentials]string{initial: `"replay":false`, reconnect: `"replay":6500`, capped: `"replay":30000`} {
		if payload := decodedPayload(t, credentials); !strings.Contains(payload, replay) {
			t.Fatalf("payload %s, want %s", payload, replay)
		}
	}
}

func TestProviderWithADoneContextNeitherCallsClaimsNorSigns(t *testing.T) {
	signer, reads := countingSigner(t)
	var calls atomic.Int64
	provider, _ := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		calls.Add(1)

		return scopedClaims(), nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := provider(ctx, celeris.CredentialRequest{}); !errors.Is(err, context.Canceled) || calls.Load() != 0 || reads.Load() != 0 {
		t.Fatalf("got %v, calls %d, reads %d", err, calls.Load(), reads.Load())
	}
}

func TestCancellationDuringClaimsPreventsSigning(t *testing.T) {
	signer, reads := countingSigner(t)
	ctx, cancel := context.WithCancel(t.Context())
	provider, _ := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		cancel()

		return scopedClaims(), nil
	})

	if _, err := provider(ctx, celeris.CredentialRequest{}); !errors.Is(err, context.Canceled) || reads.Load() != 0 {
		t.Fatalf("got %v, reads %d", err, reads.Load())
	}
}

func TestClaimsErrorPassesThroughWithoutSigning(t *testing.T) {
	signer, reads := countingSigner(t)
	failure := errors.New("synthetic-claims-failure")
	provider, _ := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		return Claims{}, failure
	})

	if _, err := provider(t.Context(), celeris.CredentialRequest{}); !errors.Is(err, failure) || reads.Load() != 0 {
		t.Fatalf("got %v, reads %d", err, reads.Load())
	}
}

func TestRequestFieldsNeverWidenTheClaims(t *testing.T) {
	signer, _ := countingSigner(t)
	provider, _ := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		return scopedClaims(), nil
	})

	credentials, err := provider(t.Context(), celeris.CredentialRequest{ChannelReference: "admin-room", Reconnect: true, ReplayLookback: time.Hour})

	if err != nil {
		t.Fatal(err)
	}

	if payload := decodedPayload(t, credentials); strings.Contains(payload, "admin-room") || !strings.Contains(payload, `"replay":false`) {
		t.Fatalf("payload %s", payload)
	}
}

func TestInvalidClaimsFromTheCallbackAreAConfigurationError(t *testing.T) {
	signer, _ := countingSigner(t)
	provider, _ := NewCredentialProvider(signer, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		return Claims{}, nil
	})

	_, err := provider(t.Context(), celeris.CredentialRequest{})
	assertConfiguration(t, err, "Invalid claims. Channels: Required. Permissions: Required.")
}

func TestProviderNeedsASignerAndClaims(t *testing.T) {
	_, err := NewCredentialProvider(nil, nil)
	assertConfiguration(t, err, "Invalid credential provider options. Signer: Required. Claims: Required.")
}
