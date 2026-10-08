package celerisserver_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
	celerisserver "github.com/useceleris/sdk-go-server"
)

func Example() {
	// From trusted configuration, never from code.
	signer, err := celerisserver.NewSigner(celerisserver.SignerOptions{
		ClientID:      os.Getenv("CELERIS_CLIENT_ID"),
		SigningSecret: os.Getenv("CELERIS_SIGNING_SECRET"),
	})

	if err != nil {
		log.Fatal(err)
	}

	// Claims your server decided for a user it authenticated.
	credentials, err := signer.Sign(celerisserver.Claims{
		Channels: celerisserver.RestrictedChannels("room-42"),
		Permissions: celerisserver.RestrictedSegments(
			celerisserver.SegmentClaim{SegmentID: "chat", Read: true, Write: true},
		),
		Reference: "user-8317",
		Replay:    celerisserver.ReplayLookback(30 * time.Second),
	})

	if err != nil {
		log.Fatal(err)
	}

	// Return them from your credential endpoint as JSON:
	// {"payload": "...", "signature": "..."}.
	_ = credentials
} // end function Example

// A backend that consumes realtime itself signs fresh claims for every
// connection attempt.
func ExampleNewCredentialProvider() {
	var signer *celerisserver.Signer // from celerisserver.NewSigner

	provider, err := celerisserver.NewCredentialProvider(signer, func(_ context.Context, request celeris.CredentialRequest) (celerisserver.Claims, error) {
		claims := celerisserver.Claims{
			Channels:    celerisserver.RestrictedChannels(request.ChannelReference),
			Permissions: celerisserver.AllSegments(true, true),
		}

		// On reconnect, catch up on what the outage missed.
		if request.Reconnect {
			claims.Replay = celerisserver.ReplayLookback(request.ReplayLookback)
		}

		return claims, nil
	})

	if err != nil {
		log.Fatal(err)
	}

	client, err := celeris.NewClient(celeris.ClientOptions{CredentialProvider: provider})

	if err != nil {
		log.Fatal(err)
	}

	_ = client
} // end function ExampleNewCredentialProvider

// Invalid claims name each field and rule, never the value.
func ExampleSigner_Sign() {
	signer, err := celerisserver.NewSigner(celerisserver.SignerOptions{ClientID: "client", SigningSecret: "secret"})

	if err != nil {
		log.Fatal(err)
	}

	_, err = signer.Sign(celerisserver.Claims{
		Channels:    celerisserver.RestrictedChannels("room-42", "room-42"),
		Permissions: celerisserver.AllSegments(true, false),
		Reference:   "user:8317",
	})

	fmt.Println(err)
	// Output: Invalid claims. Channels.References: Must not repeat a channel reference. Reference: Must not contain a colon, CR or LF.
} // end function ExampleSigner_Sign
