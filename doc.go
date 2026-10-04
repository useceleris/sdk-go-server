// Package celerisserver mints Celeris credentials on a trusted server.
//
// A [Signer] holds your application's signing secret and signs [Claims] your
// server decides for a user it has authenticated. Hand the resulting
// credentials to clients from your own credential endpoint, or let a server
// that consumes realtime itself sign per attempt with
// [NewCredentialProvider].
//
// Never ship the signing secret, or this package, to a client.
package celerisserver
