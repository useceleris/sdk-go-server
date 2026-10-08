package celerisserver

import (
	"context"

	celeris "github.com/useceleris/sdk-go-client"
)

// ClaimsFunc decides the claims for one connection attempt. It is
// authoritative: nothing in the request, which comes from the connecting
// client, widens scope beyond what it returns.
type ClaimsFunc func(ctx context.Context, request celeris.CredentialRequest) (Claims, error)

// NewCredentialProvider returns a [celeris.CredentialProvider] for a trusted
// server that consumes realtime itself. Every attempt calls claims afresh and
// signs with a fresh timestamp. A context already done, before claims runs or
// after it returns, fails without signing; claims' own errors are returned
// unchanged, and the client reports them as a transport failure.
//
// It fails with [celeris.ErrConfiguration] when signer or claims is nil.
func NewCredentialProvider(signer *Signer, claims ClaimsFunc) (celeris.CredentialProvider, error) {
	var failures []string

	if signer == nil {
		failures = append(failures, failure("Signer", "Required"))
	}

	if claims == nil {
		failures = append(failures, failure("Claims", "Required"))
	}

	if err := configurationError("credential provider options", failures); err != nil {
		return nil, err
	}

	return func(ctx context.Context, request celeris.CredentialRequest) (celeris.Credentials, error) {
		if err := ctx.Err(); err != nil {
			return celeris.Credentials{}, err
		}

		signingClaims, err := claims(ctx, request)

		if err != nil {
			return celeris.Credentials{}, err
		}

		// A cancellation that landed while claims ran still fails before
		// signing.
		if err := ctx.Err(); err != nil {
			return celeris.Credentials{}, err
		}

		return signer.Sign(signingClaims)
	}, nil
} // end function NewCredentialProvider
