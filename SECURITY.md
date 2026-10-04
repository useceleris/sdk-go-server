# Security policy

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub's [Security Advisories](https://github.com/useceleris/sdk-go-server/security/advisories/new) on this repository. That channel is private between you and the maintainer, and it lets us prepare a fix before anything is disclosed.

Include what you need to make the problem reproducible: the affected version, the Go version you saw it on, and the smallest example that shows it. A suggested fix is welcome but not required.

You should get an acknowledgement within a few days. We will tell you what we found, what we intend to do, and when we expect a fix to land, and we will credit you when it is published unless you would rather we did not.

## Supported versions

Fixes land on the latest release. There is no long-term support branch.

## Scope

In scope: anything in this module that signs credentials granting more than the claims say, any leak of the signing secret or of credentials into a place they should not reach, and any input that can crash or corrupt a consuming application.

Out of scope: the Celeris service itself, which is reported through the same channel on its own repository, and findings that require an attacker who already holds the signing secret. That secret is the trust boundary, and its compromise is total by design.

## What this module promises

This is the server module: it holds the signing secret, so it belongs on trusted servers only. A `Signer` keeps the secret only inside the digest it computes, so printing it never shows the secret; `SignerOptions` print as redacted through `fmt` and `slog`; the package prints and logs nothing; and validation errors name the failed field and rule but never its value. Every signed string is checked to be valid UTF-8, so the signed bytes are exactly what the caller passed. Signing uses only the standard library, and its output equals the JavaScript reference signer's byte for byte.
