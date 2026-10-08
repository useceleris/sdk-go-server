// Command credential-endpoint is the endpoint every browser or mobile
// application needs. It authenticates its own user and derives the authorized
// claims server-side; a permission the client asks for is never trusted.
//
// Run it with CELERIS_CLIENT_ID and CELERIS_SIGNING_SECRET set, then POST
// {"channelReference": "room-42"} with "Authorization: Bearer demo-session".
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	celerisserver "github.com/useceleris/sdk-go-server"
)

type user struct {
	id        string
	rooms     []string
	moderator bool
} // end struct user

// authenticate stands in for YOUR authentication: a session cookie, bearer
// token or your framework's user. It never comes from the request body.
func authenticate(request *http.Request) (user, bool) {
	if request.Header.Get("Authorization") != "Bearer demo-session" {
		return user{}, false
	}

	return user{id: "user-8317", rooms: []string{"room-42"}}, true
} // end function authenticate

// claimsFor decides scope on the server. A moderator may write; everyone
// else reads.
func claimsFor(account user, channelReference string, replayLookbackMS int64) celerisserver.Claims {
	return celerisserver.Claims{
		Channels: celerisserver.RestrictedChannels(channelReference),
		Permissions: celerisserver.RestrictedSegments(
			celerisserver.SegmentClaim{SegmentID: "chat", Read: true, Write: account.moderator},
			celerisserver.SegmentClaim{SegmentID: "presence", Read: true},
		),
		// The identity peers see: no colon, CR or LF.
		Reference: account.id,
		// An application policy, not a retention promise: catch up on at most
		// thirty seconds, whatever the client asked for.
		Replay: celerisserver.ReplayLookback(time.Duration(min(max(replayLookbackMS, 0), 30_000)) * time.Millisecond),
	}
} // end function claimsFor

func main() {
	signer, err := celerisserver.NewSigner(celerisserver.SignerOptions{
		ClientID:      os.Getenv("CELERIS_CLIENT_ID"),
		SigningSecret: os.Getenv("CELERIS_SIGNING_SECRET"),
	})

	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("POST /realtime-credentials", func(writer http.ResponseWriter, request *http.Request) {
		account, ok := authenticate(request)

		if !ok {
			http.Error(writer, "unauthenticated", http.StatusUnauthorized)

			return
		}

		var body struct {
			ChannelReference string `json:"channelReference"`
			ReplayLookbackMS int64  `json:"replayLookbackMs"`
		}

		if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096)).Decode(&body); err != nil {
			http.Error(writer, "invalid request", http.StatusBadRequest)

			return
		}

		// Authorize the requested channel against what the user may access.
		if !slices.Contains(account.rooms, body.ChannelReference) {
			http.Error(writer, "forbidden", http.StatusForbidden)

			return
		}

		// Sign fresh per request; never cache or backdate credentials.
		credentials, err := signer.Sign(claimsFor(account, body.ChannelReference, body.ReplayLookbackMS))

		if err != nil {
			http.Error(writer, "signing failed", http.StatusInternalServerError)

			return
		}

		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(credentials)
	})

	address := "127.0.0.1:" + os.Getenv("CELERIS_EXAMPLE_PORT")

	if address == "127.0.0.1:" {
		address = "127.0.0.1:8080"
	}

	// A header deadline keeps slow clients from holding connections open.
	server := &http.Server{Addr: address, ReadHeaderTimeout: 5 * time.Second}

	log.Println("example: credential endpoint listening on", address)
	log.Fatal(server.ListenAndServe())
} // end function main
