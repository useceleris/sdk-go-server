package celerisserver

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"time"

	celeris "github.com/useceleris/sdk-go-client"
)

// SignerOptions configures a [Signer]. They hold the signing secret, so fmt
// verbs and [slog] show them as redacted. fmt prints an unexported struct
// field field by field, which redaction cannot reach: pass the options to
// [NewSigner] rather than keeping them in a struct of your own.
type SignerOptions struct {
	// ClientID is your application's client id: nonempty, with no colon, CR
	// or LF.
	ClientID string

	// SigningSecret is your application's signing secret. Keep it on trusted
	// servers only.
	SigningSecret string

	// Clock returns the time credentials are signed at. Nil means time.Now.
	Clock func() time.Time
}

const redactedSignerOptions = "celerisserver.SignerOptions{redacted}"

func (SignerOptions) String() string {
	return redactedSignerOptions
}

// Format redacts the options for every fmt verb.
func (SignerOptions) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, redactedSignerOptions)
}

// LogValue redacts the options in [slog] output.
func (SignerOptions) LogValue() slog.Value {
	return slog.StringValue("redacted")
}

// Signer mints credentials on a trusted server. It keeps the signing secret
// only inside the digest it computes, so printing a Signer, even field by
// field, never shows the secret. It is safe for concurrent use.
type Signer struct {
	clientID string
	digest   func(message []byte) []byte
	clock    func() time.Time
}

// NewSigner validates options and returns a signer. It fails with
// [celeris.ErrConfiguration], naming the field and rule but never the value.
func NewSigner(options SignerOptions) (*Signer, error) {
	if err := validateSignerOptions(options); err != nil {
		return nil, err
	}

	clock := options.Clock

	if clock == nil {
		clock = time.Now
	}

	key := []byte(options.SigningSecret)

	digest := func(message []byte) []byte {
		mac := hmac.New(sha512.New, key)
		mac.Write(message)

		return mac.Sum(nil)
	}

	return &Signer{clientID: options.ClientID, digest: digest, clock: clock}, nil
}

// Sign mints credentials for claims, timestamped now. Sign fresh for every
// connection attempt; never cache or reuse credentials (D-001). It fails with
// [celeris.ErrConfiguration] for invalid claims, naming each field and rule.
func (signer *Signer) Sign(claims Claims) (celeris.Credentials, error) {
	if err := validateClaims(claims); err != nil {
		return celeris.Credentials{}, err
	}

	moment := signer.clock()

	// Seconds first: UnixMilli is undefined for times outside its range, which
	// could otherwise wrap into the valid window.
	seconds := moment.Unix()
	timestamp := moment.UnixMilli()

	if seconds < 0 || seconds > maximumTimestamp/1000 || timestamp < 1 || timestamp > maximumTimestamp {
		return celeris.Credentials{}, &celeris.Error{Code: celeris.ErrConfiguration, Message: "Invalid timestamp from Clock. Must be between 1 and 253402300799999 milliseconds."}
	}

	payload := base64.StdEncoding.EncodeToString(appendTokenPayload(nil, timestamp, claims))

	// The digest covers the encoded payload text, as the server checks it.
	digest := hex.EncodeToString(signer.digest([]byte(payload)))
	signature := base64.StdEncoding.EncodeToString([]byte(signer.clientID + ":" + digest))

	return celeris.Credentials{Payload: payload, Signature: signature}, nil
}
