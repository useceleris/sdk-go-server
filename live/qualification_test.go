package live

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
	celerisserver "github.com/useceleris/sdk-go-server"
)

func TestConnectsWithServerSignedCredentials(t *testing.T) {
	reference := uniqueChannelReference("auth")
	channel := newChannel(t, qualificationClient(t, signerOptions(), fixedClaims(allPermissionClaims(reference))), reference)
	greetings := make(chan string, 8)
	channel.Events().OnNotice(func(notice celeris.ServerNotice) { greetings <- string(notice.Payload) })

	if err := channel.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}

	select {
	case greeting := <-greetings:
		if !strings.Contains(greeting, "Successfully connected") {
			t.Fatalf("greeting %q", greeting)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no greeting")
	}
} // end function TestConnectsWithServerSignedCredentials

// DEV-02: a refused handshake is reported as a transport failure.
func TestRejectsAWrongSecretAndAnUnknownClientAsTransport(t *testing.T) {
	reference := uniqueChannelReference("reject")
	wrongSecret := signerOptions()
	wrongSecret.SigningSecret = "wrong-signing-secret"
	unknownClient := signerOptions()
	unknownClient.ClientID = "goqual-unknown"

	for _, options := range []celerisserver.SignerOptions{wrongSecret, unknownClient} {
		channel := newChannel(t, qualificationClient(t, options, fixedClaims(allPermissionClaims(reference))), reference)
		err := channel.Connect(t.Context())

		if !errors.Is(err, celeris.ErrTransport) || channel.State() != celeris.StateFailed || strings.Contains(err.Error(), "wrong-signing-secret") {
			t.Fatalf("got %v, state %s", err, channel.State())
		}
	}
} // end function TestRejectsAWrongSecretAndAnUnknownClientAsTransport

// D-001: the server accepts up to the observed 60-minute window against a
// documented 60-second intent; recorded, not relied upon.
func TestRejectsExpiredAndFutureClocksInsideTheObservedWindow(t *testing.T) {
	reference := uniqueChannelReference("window")

	for _, offset := range []time.Duration{-61 * time.Minute, 5 * time.Minute} {
		options := signerOptions()
		options.Clock = func() time.Time { return time.Now().Add(offset) }
		channel := newChannel(t, qualificationClient(t, options, fixedClaims(allPermissionClaims(reference))), reference)

		if err := channel.Connect(t.Context()); !errors.Is(err, celeris.ErrTransport) {
			t.Fatalf("offset %v: got %v", offset, err)
		}
	}

	options := signerOptions()
	options.Clock = func() time.Time { return time.Now().Add(-59 * time.Minute) }
	stale := newChannel(t, qualificationClient(t, options, fixedClaims(allPermissionClaims(reference))), reference)

	if err := stale.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}
} // end function TestRejectsExpiredAndFutureClocksInsideTheObservedWindow

func TestEnforcesTheTokensChannelRestriction(t *testing.T) {
	allowed := uniqueChannelReference("scope-allowed")
	denied := newChannel(t, qualificationClient(t, signerOptions(), fixedClaims(allPermissionClaims(allowed))), uniqueChannelReference("scope-denied"))

	if err := denied.Connect(t.Context()); !errors.Is(err, celeris.ErrTransport) {
		t.Fatalf("got %v", err)
	}

	connectedChannel(t, allowed, fixedClaims(allPermissionClaims(allowed)))
} // end function TestEnforcesTheTokensChannelRestriction

func TestRoundTripsAPayloadWithItsIDAndNoSelfEcho(t *testing.T) {
	reference := uniqueChannelReference("msg")
	claims := fixedClaims(allPermissionClaims(reference))
	publisher := connectedChannel(t, reference, claims)
	receiver := connectedChannel(t, reference, claims)
	echoes := make(chan string, 4)
	segment(t, publisher, "chat").OnMessage(func(_ []byte, metadata celeris.MessageMetadata) { echoes <- metadata.MessageID })
	subscribe(t, segment(t, publisher, "chat"))
	subscribe(t, segment(t, receiver, "chat"))
	time.Sleep(1500 * time.Millisecond)

	if err := segment(t, publisher, "chat").Publish(t.Context(), []byte("hello-서버")); err != nil {
		t.Fatal(err)
	}

	message := nextMessage(t, segment(t, receiver, "chat"), func(delivery) bool { return true }, "cross-connection delivery", 15*time.Second)

	if !generatedMessageID.MatchString(message.metadata.MessageID) || string(message.payload) != "hello-서버" {
		t.Fatalf("delivery %+v", message)
	}

	select {
	case identifier := <-echoes:
		t.Fatalf("publisher echoed %s without AllowEcho", identifier)
	case <-time.After(1500 * time.Millisecond):
	}
} // end function TestRoundTripsAPayloadWithItsIDAndNoSelfEcho

func TestEchoesToThePublisherWhenClaimsAllowEcho(t *testing.T) {
	reference := uniqueChannelReference("echo")
	claims := allPermissionClaims(reference)
	claims.AllowEcho = true
	channel := connectedChannel(t, reference, fixedClaims(claims))
	chat := segment(t, channel, "chat")
	subscribe(t, chat)
	time.Sleep(1500 * time.Millisecond)

	if err := chat.Publish(t.Context(), []byte("self")); err != nil {
		t.Fatal(err)
	}

	nextMessage(t, chat, func(message delivery) bool { return string(message.payload) == "self" }, "an echoed publish", 15*time.Second)
} // end function TestEchoesToThePublisherWhenClaimsAllowEcho

func TestDeniesAReadOnlyRestrictedSegmentsPublish(t *testing.T) {
	reference := uniqueChannelReference("perm")
	readOnly := connectedChannel(t, reference, fixedClaims(celerisserver.Claims{
		Channels:    celerisserver.RestrictedChannels(reference),
		Permissions: celerisserver.RestrictedSegments(celerisserver.SegmentClaim{SegmentID: "chat", Read: true}),
	}))

	// Publish completes locally; the denial arrives later, naming the segment.
	if err := segment(t, readOnly, "chat").Publish(t.Context(), []byte("denied")); err != nil {
		t.Fatal(err)
	}

	denial := waitFor(t, readOnly.Events().OnError, serverErrorOfType(celeris.PermissionDeniedError), "the PermissionDeniedError frame", 15*time.Second)

	serverError, ok := errors.AsType[*celeris.ServerError](denial)

	if !ok || serverError.SubType != "PUB" || serverError.Resource != "chat" || readOnly.State() != celeris.StateConnected {
		t.Fatalf("denial %v", denial)
	}
} // end function TestDeniesAReadOnlyRestrictedSegmentsPublish

func TestReplaysRecentMessagesThroughReconnectClaims(t *testing.T) {
	reference := uniqueChannelReference("replay")
	claims := allPermissionClaims(reference)
	publisher := connectedChannel(t, reference, fixedClaims(claims))
	liveReceiver := connectedChannel(t, reference, fixedClaims(claims))
	var mutex sync.Mutex
	var liveIDs []string
	segment(t, liveReceiver, "history").OnMessage(func(_ []byte, metadata celeris.MessageMetadata) {
		mutex.Lock()
		liveIDs = append(liveIDs, metadata.MessageID)
		mutex.Unlock()
	})

	subscribe(t, segment(t, liveReceiver, "history"))
	time.Sleep(1500 * time.Millisecond)

	for _, body := range []string{"one", "two", "three"} {
		_ = segment(t, publisher, "history").Publish(t.Context(), []byte(body))
	}

	nextMessage(t, segment(t, liveReceiver, "history"), func(delivery) bool {
		mutex.Lock()
		defer mutex.Unlock()

		return len(liveIDs) >= 3
	}, "the three live deliveries", 20*time.Second)

	liveReceiver.Close()

	// The claims function applies the request's replay lookback, or a minute
	// on a first connect.
	replaying := func(_ context.Context, request celeris.CredentialRequest) (celerisserver.Claims, error) {
		replay := claims
		replay.Replay = celerisserver.ReplayLookback(time.Minute)

		if request.Reconnect {
			replay.Replay = celerisserver.ReplayLookback(request.ReplayLookback)
		}

		return replay, nil
	}

	replayReceiver := connectedChannel(t, reference, replaying)
	replayed := map[string]string{}
	history := segment(t, replayReceiver, "history")
	history.OnMessage(func(payload []byte, metadata celeris.MessageMetadata) {
		mutex.Lock()
		replayed[string(payload)] = metadata.MessageID
		mutex.Unlock()
	})

	subscribe(t, history)
	nextMessage(t, history, func(delivery) bool {
		mutex.Lock()
		defer mutex.Unlock()

		return len(replayed) >= 3
	}, "the replayed history", 25*time.Second)

	mutex.Lock()
	defer mutex.Unlock()

	for index, body := range []string{"one", "two", "three"} {
		if replayed[body] != liveIDs[index] {
			t.Fatalf("%s replayed as %q, live id %q", body, replayed[body], liveIDs[index])
		}
	}
} // end function TestReplaysRecentMessagesThroughReconnectClaims

func TestSurfacesPresenceForAServerSignedReference(t *testing.T) {
	reference := uniqueChannelReference("presence")
	claims := allPermissionClaims(reference)
	watcher := connectedChannel(t, reference, fixedClaims(claims))
	watched := segment(t, watcher, "chat")

	if _, err := watched.SubscribePresence(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1500 * time.Millisecond)
	actorClaims := claims
	actorClaims.Reference = "goqual-server-actor"
	actor := connectedChannel(t, reference, fixedClaims(actorClaims))
	subscribe(t, segment(t, actor, "chat"))

	join := waitFor(t, watched.OnPresence, func(event celeris.PresenceEvent) bool {
		return event.Joined && event.TokenReference == "goqual-server-actor"
	}, "the actor's presence join event", 20*time.Second)

	if join.SegmentID != "chat" || join.ConnectionID == "" {
		t.Fatalf("join %+v", join)
	}

	page, err := watched.PresenceList(t.Context(), 1, 10)

	if err != nil {
		t.Fatal(err)
	}

	found := false

	for _, connection := range page.Connections {
		found = found || connection.TokenReference == "goqual-server-actor"
	}

	if page.Total < 1 || !found {
		t.Fatalf("page %+v", page)
	}
} // end function TestSurfacesPresenceForAServerSignedReference
