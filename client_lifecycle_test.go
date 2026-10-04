package celerisserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	celeris "github.com/useceleris/sdk-go-client"
)

// These tests drive the real client with the provider, against an in-process
// WebSocket server, in real time.

func lifecycleServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	var handshakes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		handshakes.Add(1)
		conn, err := websocket.Accept(writer, request, nil)

		if err != nil {
			return
		}

		defer func() { _ = conn.CloseNow() }()

		for {
			if _, _, err := conn.Read(context.Background()); err != nil {
				return
			}
		}
	}))

	t.Cleanup(server.Close)

	return server, &handshakes
}

func lifecycleChannel(t *testing.T, server *httptest.Server, claims ClaimsFunc) *celeris.Channel {
	t.Helper()

	signer, _ := countingSigner(t)
	provider, err := NewCredentialProvider(signer, claims)

	if err != nil {
		t.Fatal(err)
	}

	client, err := celeris.NewClient(celeris.ClientOptions{
		CredentialProvider:    provider,
		BaseURL:               "ws" + strings.TrimPrefix(server.URL, "http"),
		AllowInsecureLoopback: true,
		ConnectTimeout:        5 * time.Second,
	})

	if err != nil {
		t.Fatal(err)
	}

	channel, err := client.Channel("room-1")

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(channel.Close)

	return channel
}

func TestProviderConnectsTheRealClient(t *testing.T) {
	server, handshakes := lifecycleServer(t)
	channel := lifecycleChannel(t, server, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		return scopedClaims(), nil
	})

	if err := channel.Connect(t.Context()); err != nil || channel.State() != celeris.StateConnected || handshakes.Load() != 1 {
		t.Fatalf("got %v, state %s, %d handshakes", err, channel.State(), handshakes.Load())
	}
}

func TestCloseDuringClaimsCancelsWithoutAHandshake(t *testing.T) {
	server, handshakes := lifecycleServer(t)
	entered := make(chan struct{})
	cancelled := make(chan error, 1)
	channel := lifecycleChannel(t, server, func(ctx context.Context, _ celeris.CredentialRequest) (Claims, error) {
		close(entered)
		<-ctx.Done()
		cancelled <- ctx.Err()

		return scopedClaims(), nil
	})

	result := make(chan error, 1)

	go func() { result <- channel.Connect(t.Context()) }()

	<-entered
	channel.Close()

	if err := <-result; !errors.Is(err, celeris.ErrCancelled) {
		t.Fatalf("got %v", err)
	}

	if err := <-cancelled; err == nil || handshakes.Load() != 0 {
		t.Fatalf("claims context %v, %d handshakes", err, handshakes.Load())
	}
}

func TestCallerCancellationDuringClaimsFailsWithoutASocket(t *testing.T) {
	server, handshakes := lifecycleServer(t)
	entered := make(chan struct{})
	channel := lifecycleChannel(t, server, func(ctx context.Context, _ celeris.CredentialRequest) (Claims, error) {
		close(entered)
		<-ctx.Done()

		return scopedClaims(), nil
	})

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)

	go func() { result <- channel.Connect(ctx) }()

	<-entered
	cancel()

	if err := <-result; !errors.Is(err, celeris.ErrCancelled) || channel.State() != celeris.StateFailed || handshakes.Load() != 0 {
		t.Fatalf("got %v, state %s, %d handshakes", err, channel.State(), handshakes.Load())
	}
}

func TestClaimsFailureSurfacesOnlyAFixedSafeError(t *testing.T) {
	server, _ := lifecycleServer(t)
	channel := lifecycleChannel(t, server, func(context.Context, celeris.CredentialRequest) (Claims, error) {
		return Claims{}, errorString("synthetic-secret claims failure")
	})

	err := channel.Connect(t.Context())

	if !errors.Is(err, celeris.ErrTransport) || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("got %v", err)
	}
}

type errorString string

func (message errorString) Error() string {
	return string(message)
}
