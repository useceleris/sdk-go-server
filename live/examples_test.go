package live

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
)

// exampleEnvironment is the environment for go run in the repository root.
// It drops GOWORK, so a local workspace can stand in for a client version
// that is not published yet.
func exampleEnvironment(extra ...string) []string {
	var environment []string

	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GOWORK=") {
			environment = append(environment, entry)
		}
	}

	return append(environment, extra...)
} // end function exampleEnvironment

func TestRunsTheQuickstart(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "run", "./examples/quickstart")
	command.Dir = ".."
	command.Env = exampleEnvironment()
	output, err := command.CombinedOutput()

	if err != nil {
		t.Fatalf("quickstart failed: %v\n%s", err, output)
	}

	if !regexp.MustCompile(`example: ok delivered=[1-9]\d*`).Match(output) {
		t.Fatalf("output:\n%s", output)
	}
} // end function TestRunsTheQuickstart

// The credential endpoint authenticates and authorizes server-side, and its
// credentials drive a real connection: the demo user reads "chat" but may not
// write to it.
func TestMintsCredentialsThroughTheEndpointAndConnectsWithThem(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")

	if err != nil {
		t.Fatal(err)
	}

	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	_ = listener.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	command := exec.CommandContext(ctx, "go", "run", "./examples/credential-endpoint")
	command.Dir = ".."
	command.Env = exampleEnvironment("CELERIS_EXAMPLE_PORT=" + port)
	stderr, err := command.StderrPipe()

	if err != nil {
		t.Fatal(err)
	}

	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cancel()
		_ = command.Wait()
	})

	// The endpoint logs one line once it listens.
	lines := bufio.NewScanner(stderr)

	if !lines.Scan() || !strings.Contains(lines.Text(), "listening") {
		t.Fatalf("the endpoint did not start: %q", lines.Text())
	}

	endpoint := "http://127.0.0.1:" + port + "/realtime-credentials"

	post := func(authorization string, request map[string]any) (int, []byte) {
		body, err := json.Marshal(request)

		if err != nil {
			t.Fatal(err)
		}

		httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(body))

		if err != nil {
			t.Fatal(err)
		}

		httpRequest.Header.Set("Authorization", authorization)
		response, err := http.DefaultClient.Do(httpRequest)

		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = response.Body.Close() }()

		var buffer bytes.Buffer
		_, _ = buffer.ReadFrom(response.Body)

		return response.StatusCode, buffer.Bytes()
	}

	if status, _ := post("Bearer wrong", map[string]any{"channelReference": "room-42"}); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request answered %d", status)
	}

	if status, _ := post("Bearer demo-session", map[string]any{"channelReference": "other-room"}); status != http.StatusForbidden {
		t.Fatalf("unauthorized channel answered %d", status)
	}

	oversized := map[string]any{"channelReference": "room-42", "padding": strings.Repeat("x", 4096)}

	if status, _ := post("Bearer demo-session", oversized); status != http.StatusBadRequest {
		t.Fatalf("oversized request answered %d", status)
	}

	// A lookback the client asks for is clamped by the endpoint's policy, not
	// refused.
	if status, _ := post("Bearer demo-session", map[string]any{"channelReference": "room-42", "replayLookbackMs": -1}); status != http.StatusOK {
		t.Fatalf("negative lookback answered %d", status)
	}

	status, body := post("Bearer demo-session", map[string]any{"channelReference": "room-42"})

	if status != http.StatusOK {
		t.Fatalf("authorized request answered %d", status)
	}

	var credentials celeris.Credentials

	if err := json.Unmarshal(body, &credentials); err != nil {
		t.Fatal(err)
	}

	client, err := celeris.NewClient(celeris.ClientOptions{
		CredentialProvider:    func(context.Context, celeris.CredentialRequest) (celeris.Credentials, error) { return credentials, nil },
		BaseURL:               websocketURL(),
		AllowInsecureLoopback: true,
	})

	if err != nil {
		t.Fatal(err)
	}

	reader := newChannel(t, client, "room-42")

	if err := reader.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}

	subscribe(t, segment(t, reader, "chat"))
	time.Sleep(1500 * time.Millisecond)

	if err := segment(t, reader, "chat").Publish(t.Context(), []byte("should be denied")); err != nil {
		t.Fatal(err)
	}

	_ = waitFor(t, reader.Events().OnError, serverErrorOfType(celeris.PermissionDeniedError), "the PermissionDeniedError frame", 15*time.Second)

	if reader.State() != celeris.StateConnected {
		t.Fatalf("state %s", reader.State())
	}
} // end function TestMintsCredentialsThroughTheEndpointAndConnectsWithThem
