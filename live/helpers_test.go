package live

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
	celerisserver "github.com/useceleris/sdk-go-server"
)

var channelCounter atomic.Int64

// TestMain fails the run at once when the target cannot be used. Missing
// configuration fails; it never skips.
func TestMain(m *testing.M) {
	loadEnvironment("../.env")

	for _, name := range []string{"CELERIS_WS_URL", "CELERIS_CLIENT_ID", "CELERIS_SIGNING_SECRET"} {
		if os.Getenv(name) == "" {
			fmt.Fprintln(os.Stderr, name+" not set. Put CELERIS_WS_URL, CELERIS_CLIENT_ID and CELERIS_SIGNING_SECRET in the repository's .env (gitignored) or the environment.")
			os.Exit(1)
		}
	}

	target, err := url.Parse(websocketURL())

	if err != nil {
		fmt.Fprintln(os.Stderr, "CELERIS_WS_URL is not a URL.")
		os.Exit(1)
	}

	port := target.Port()

	if port == "" {
		port = map[string]string{"wss": "443", "ws": "80"}[target.Scheme]
	}

	probe, err := net.DialTimeout("tcp", net.JoinHostPort(target.Hostname(), port), 5*time.Second)

	if err != nil {
		fmt.Fprintln(os.Stderr, "The realtime service at "+target.Host+" is not reachable. Start the stack, or point CELERIS_WS_URL elsewhere.")
		os.Exit(1)
	}

	_ = probe.Close()
	os.Exit(m.Run())
} // end function TestMain

func loadEnvironment(path string) {
	file, err := os.Open(path)

	if err != nil {
		return
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, found := strings.Cut(line, "=")

		if !found || strings.HasPrefix(line, "#") {
			continue
		}

		key = strings.TrimSpace(key)

		if _, set := os.LookupEnv(key); !set {
			_ = os.Setenv(key, strings.Trim(strings.TrimSpace(value), `'"`))
		}
	}
} // end function loadEnvironment

func websocketURL() string {
	return os.Getenv("CELERIS_WS_URL")
} // end function websocketURL

func uniqueChannelReference(label string) string {
	return "goqual-server-" + label + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10) + "-" + strconv.FormatInt(channelCounter.Add(1), 10)
} // end function uniqueChannelReference

// allPermissionClaims grants every segment of one channel.
func allPermissionClaims(reference string) celerisserver.Claims {
	return celerisserver.Claims{
		Channels:    celerisserver.RestrictedChannels(reference),
		Permissions: celerisserver.AllSegments(true, true),
	}
} // end function allPermissionClaims

// signerOptions are the qualification credentials, which a test may change.
func signerOptions() celerisserver.SignerOptions {
	return celerisserver.SignerOptions{ClientID: os.Getenv("CELERIS_CLIENT_ID"), SigningSecret: os.Getenv("CELERIS_SIGNING_SECRET")}
} // end function signerOptions

func qualificationClient(t *testing.T, options celerisserver.SignerOptions, claims celerisserver.ClaimsFunc) *celeris.Client {
	t.Helper()

	signer, err := celerisserver.NewSigner(options)

	if err != nil {
		t.Fatal(err)
	}

	provider, err := celerisserver.NewCredentialProvider(signer, claims)

	if err != nil {
		t.Fatal(err)
	}

	client, err := celeris.NewClient(celeris.ClientOptions{CredentialProvider: provider, BaseURL: websocketURL(), AllowInsecureLoopback: true})

	if err != nil {
		t.Fatal(err)
	}

	return client
} // end function qualificationClient

func fixedClaims(claims celerisserver.Claims) celerisserver.ClaimsFunc {
	return func(context.Context, celeris.CredentialRequest) (celerisserver.Claims, error) {
		return claims, nil
	}
} // end function fixedClaims

func newChannel(t *testing.T, client *celeris.Client, reference string) *celeris.Channel {
	t.Helper()

	channel, err := client.Channel(reference)

	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(channel.Close)

	return channel
} // end function newChannel

func connectedChannel(t *testing.T, reference string, claims celerisserver.ClaimsFunc) *celeris.Channel {
	t.Helper()

	channel := newChannel(t, qualificationClient(t, signerOptions(), claims), reference)

	if err := channel.Connect(t.Context()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	return channel
} // end function connectedChannel

func segment(t *testing.T, channel *celeris.Channel, segmentID string) *celeris.Segment {
	t.Helper()

	handle, err := channel.Segment(segmentID)

	if err != nil {
		t.Fatal(err)
	}

	return handle
} // end function segment

func subscribe(t *testing.T, handle *celeris.Segment) {
	t.Helper()

	if _, err := handle.Subscribe(); err != nil {
		t.Fatal(err)
	}
} // end function subscribe

type delivery struct {
	payload  []byte
	metadata celeris.MessageMetadata
} // end struct delivery

func waitFor[Value any](t *testing.T, register func(func(Value)) func(), predicate func(Value) bool, description string, timeout time.Duration) Value {
	t.Helper()

	arrived := make(chan Value, 1)
	var once sync.Once
	remove := register(func(value Value) {
		if predicate(value) {
			once.Do(func() { arrived <- value })
		}
	})

	defer remove()

	select {
	case value := <-arrived:
		return value
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for %s", description)

		var zero Value

		return zero
	}
} // end function waitFor

func nextMessage(t *testing.T, handle *celeris.Segment, predicate func(delivery) bool, description string, timeout time.Duration) delivery {
	t.Helper()

	return waitFor(t, func(deliver func(delivery)) func() {
		return handle.OnMessage(func(payload []byte, metadata celeris.MessageMetadata) { deliver(delivery{payload, metadata}) })
	}, predicate, description, timeout)
} // end function nextMessage

func serverErrorOfType(errorType celeris.ServerErrorType) func(error) bool {
	return func(err error) bool {
		serverError, ok := errors.AsType[*celeris.ServerError](err)

		return ok && serverError.Type == errorType
	}
} // end function serverErrorOfType
