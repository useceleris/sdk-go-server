# Celeris Go server

[![Go Reference](https://pkg.go.dev/badge/github.com/useceleris/sdk-go-server.svg)](https://pkg.go.dev/github.com/useceleris/sdk-go-server)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Mints Celeris credentials on a trusted server. Your server authenticates its user, decides what they may do, and signs those claims; clients connect with the result through [sdk-go-client](https://github.com/useceleris/sdk-go-client) or any other Celeris SDK.

## How it fits

1. Your application asks your credential endpoint for credentials.
2. The endpoint authenticates the user, decides the claims, and signs them with this module.
3. The application connects with what came back. It never sees the signing secret.

A trusted backend that consumes realtime itself skips the endpoint and signs its own credentials; see [A backend that consumes realtime](#a-backend-that-consumes-realtime).

## Install

```sh
go get github.com/useceleris/sdk-go-server
```

Go 1.27 or newer. The package name is `celerisserver`. It requires the client module for the credential types and errors, never the other way round, and signs with the standard library only.

## Sign credentials

```go
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
```

`Sign` is synchronous and stamps the current time. Sign afresh for every request: credentials are never cached, reused or backdated. `celeris.Credentials` encodes as JSON, `{"payload": "...", "signature": "..."}`, ready to return from your endpoint. A `Signer` is safe for concurrent use and holds no resources.

## Claims

Authenticate and authorize the user before choosing their claims, and never sign permissions a client asked for. The zero `Claims` is invalid, so unrestricted access is always explicit:

| Field         | Values                                                                                        | Default   |
| ------------- | --------------------------------------------------------------------------------------------- | --------- |
| `Channels`    | `AllChannels()` or `RestrictedChannels(references...)`: 1–255 ASCII letters, digits, `-`, `_` | required  |
| `Permissions` | `AllSegments(read, write)` or `RestrictedSegments(claims...)`; none listed denies all         | required  |
| `Reference`   | the identity peers see in deliveries and presence: nonempty, no colon, CR or LF              | omitted   |
| `Replay`      | `ReplayBacklog()` for all the server retains, or `ReplayLookback(d)` in whole milliseconds, 0–4294967295 ms | no replay |
| `AllowEcho`   | whether a connection receives its own publishes                                               | `false`   |

Replay applies each time the connection joins a segment. `ReplayLookback(0)` and no replay differ: the first replays nothing older than the join.

## A credential endpoint

Every browser or mobile application needs one. This handler, from [examples/credential-endpoint](examples/credential-endpoint/main.go), authenticates the user, reads a bounded request, authorizes the channel and signs fresh:

```go
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
```

`authenticate` and `claimsFor` are yours: your session check, and your policy for what the user may do, including how much replay a reconnecting client may have. Serve the endpoint from an `http.Server` with `ReadHeaderTimeout` set.

## A backend that consumes realtime

`NewCredentialProvider` turns a signer into the client's `CredentialProvider`, for a trusted server that connects itself. Every attempt calls your claims function afresh and signs with a fresh timestamp; nothing in the request widens what it returns.

```go
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
```

Pass `provider` as `celeris.ClientOptions.CredentialProvider`. [examples/quickstart](examples/quickstart/main.go) is the whole flow as a program. If the claims function fails or returns invalid claims, the client's `Connect` reports a generic `ErrTransport`; sign the claims directly with `Sign` to see which rule failed.

## Errors

Invalid options or claims fail with `celeris.ErrConfiguration`, naming every field and rule that failed but never the value:

```go
_, err = signer.Sign(celerisserver.Claims{
	Channels:    celerisserver.RestrictedChannels("room-42", "room-42"),
	Permissions: celerisserver.AllSegments(true, false),
	Reference:   "user:8317",
})

fmt.Println(err)
// Output: Invalid claims. Channels.References: Must not repeat a channel reference. Reference: Must not contain a colon, CR or LF.
```

Every string that is signed must be valid UTF-8. The signed bytes equal the JavaScript reference signer's for the same claims, proven by shared signing vectors.

## Keeping the secret

The signing secret is the trust boundary: keep it on trusted servers, and never ship it, or this module, in an application. A `Signer` keeps the secret only inside the digest it computes, so printing it never shows the secret. `SignerOptions` print as redacted through `fmt` and `slog`, but `fmt` prints a struct's unexported fields one by one, so pass the options to `NewSigner` rather than keeping them in a struct of your own. The package prints and logs nothing.

## Versioning

Releases follow semantic versioning. Before v1.0.0, a minor release may change the API.

## More documentation

- [Package reference](https://pkg.go.dev/github.com/useceleris/sdk-go-server), with runnable examples
- [Go server guide](https://useceleris.com/docs/sdks/go/server) and [Go server API reference](https://useceleris.com/docs/api-reference/go-server)
- [Authentication](https://useceleris.com/docs/getting-started/authentication), for how credentials work across SDKs

## Development

`make check` runs formatting, `go vet`, golangci-lint, govulncheck, module tidiness and the unit suites under the race detector, including the signing vectors and the package checks, which also compile every snippet in this README. `make live` runs the acceptance suites against a real Celeris stack; they read `CELERIS_WS_URL`, `CELERIS_CLIENT_ID` and `CELERIS_SIGNING_SECRET` from a gitignored `.env` or the environment.

Read [CONVENTIONS.md](CONVENTIONS.md) before contributing, and [SECURITY.md](SECURITY.md) before reporting a vulnerability.

## License

[Apache 2.0](LICENSE).
