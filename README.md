# Celeris Go server

Mints Celeris credentials on a trusted server. Your server authenticates its user, decides what they may do, and signs those claims; clients connect with the result through [sdk-go-client](https://github.com/useceleris/sdk-go-client) or any other Celeris client.

## Install

```sh
go get github.com/useceleris/sdk-go-server
```

Go 1.27 or newer. The package name is `celerisserver`; it depends on the client module for the credential types and errors, never the other way round.

## Sign credentials

```go
signer, err := celerisserver.NewSigner(celerisserver.SignerOptions{
	ClientID:      "your-client-id",
	SigningSecret: "your-signing-secret", // from trusted configuration, never from code
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

Return `credentials` from your credential endpoint as JSON (`celeris.Credentials` encodes as `{"payload": ..., "signature": ...}`), and sign afresh for every request: credentials are never cached or reused. [examples/credential-endpoint](examples/credential-endpoint/main.go) is a complete `net/http` endpoint.

Authenticate and authorize users before choosing their claims; never sign permissions a client asked for. The zero `Claims` is invalid, so unrestricted access is always explicit:

| Field         | Values                                                                                     | Default            |
| ------------- | ------------------------------------------------------------------------------------------ | ------------------ |
| `Channels`    | `AllChannels()` or `RestrictedChannels(references...)`: 1–255 ASCII letters, digits, `-`, `_` | required           |
| `Permissions` | `AllSegments(read, write)` or `RestrictedSegments(claims...)`; none listed denies all     | required           |
| `Reference`   | the identity peers see: nonempty, no colon, CR or LF                                       | omitted            |
| `Replay`      | `ReplayBacklog()` or `ReplayLookback(d)`: whole milliseconds, 0–4294967295 ms             | no replay          |
| `AllowEcho`   | whether a connection receives its own publishes                                            | `false`            |

Invalid options or claims fail with `celeris.ErrConfiguration`, naming each field and rule but never the value. The signed bytes equal the JavaScript reference signer's for the same claims, proven by shared vectors.

## A backend that consumes realtime

`NewCredentialProvider` turns a signer into the client's `CredentialProvider` for a trusted server that connects itself. Every attempt calls your claims function afresh and signs with a fresh timestamp; nothing in the request widens what it returns. [examples/quickstart](examples/quickstart/main.go) shows the whole flow.

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

## Keeping the secret

The signing secret is the trust boundary: keep it on trusted servers, and never ship it or this module to a client. A `Signer` keeps the secret only inside the digest it computes, so printing it never shows the secret; `SignerOptions` print as redacted through `fmt` and `slog`, so pass them to `NewSigner` rather than keeping them around. The package prints and logs nothing. Signing uses only the standard library.

## Development

`make check` runs formatting, `go vet`, golangci-lint, govulncheck, module tidiness and the unit suites under the race detector, including the signing vectors and the package checks. `make live` runs the acceptance suites against a real Celeris stack; they read `CELERIS_WS_URL`, `CELERIS_CLIENT_ID` and `CELERIS_SIGNING_SECRET` from a gitignored `.env` or the environment.

Read [CONVENTIONS.md](CONVENTIONS.md) before contributing, and [SECURITY.md](SECURITY.md) before reporting a vulnerability.

## License

[Apache 2.0](LICENSE).
