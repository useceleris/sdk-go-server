// Command quickstart is a trusted backend process that signs its own
// credentials and consumes realtime through the client: the pattern for a
// backend worker, not a browser. The signing secret never leaves this
// process.
//
// Run it with CELERIS_WS_URL, CELERIS_CLIENT_ID and CELERIS_SIGNING_SECRET
// set.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
	celerisserver "github.com/useceleris/sdk-go-server"
)

// claimsFor decides fresh claims for every connection attempt. It is
// authoritative: nothing in the request widens its scope.
func claimsFor(_ context.Context, request celeris.CredentialRequest) (celerisserver.Claims, error) {
	claims := celerisserver.Claims{
		Channels:    celerisserver.RestrictedChannels(request.ChannelReference),
		Permissions: celerisserver.AllSegments(true, true),
		Reference:   "server-quickstart",
		// This connection sees its own publishes.
		AllowEcho: true,
	}

	// On reconnect, catch up on what the outage missed.
	if request.Reconnect {
		claims.Replay = celerisserver.ReplayLookback(request.ReplayLookback)
	}

	return claims, nil
}

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	signer, err := celerisserver.NewSigner(celerisserver.SignerOptions{
		ClientID:      os.Getenv("CELERIS_CLIENT_ID"),
		SigningSecret: os.Getenv("CELERIS_SIGNING_SECRET"),
	})

	if err != nil {
		return err
	}

	provider, err := celerisserver.NewCredentialProvider(signer, claimsFor)

	if err != nil {
		return err
	}

	client, err := celeris.NewClient(celeris.ClientOptions{
		CredentialProvider: provider,
		BaseURL:            os.Getenv("CELERIS_WS_URL"),
		// A local ws:// stack; production uses the built-in wss:// endpoint.
		AllowInsecureLoopback: true,
	})

	if err != nil {
		return err
	}

	channel, err := client.Channel("server-quickstart-" + strconv.FormatInt(time.Now().UnixMilli(), 10))

	if err != nil {
		return err
	}

	defer channel.Close()

	channel.Events().OnError(func(err error) { fmt.Println("error:", err) })

	if err := channel.Connect(ctx); err != nil {
		return err
	}

	jobs, err := channel.Segment("jobs")

	if err != nil {
		return err
	}

	var delivered atomic.Int32

	jobs.OnMessage(func(payload []byte, _ celeris.MessageMetadata) {
		fmt.Println("received", string(payload))
		delivered.Add(1)
	})

	membership, err := jobs.Subscribe()

	if err != nil {
		return err
	}

	defer membership.Cancel()

	time.Sleep(time.Second)

	if err := jobs.Publish(ctx, celeris.TextPayload("job-1 done")); err != nil {
		return err
	}

	for range 60 {
		if delivered.Load() > 0 {
			break
		}

		time.Sleep(250 * time.Millisecond)
	}

	fmt.Printf("example: ok delivered=%d\n", delivered.Load())

	return nil
}
