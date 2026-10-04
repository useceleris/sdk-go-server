# Conventions

Simplicity and maintainability are paramount. These rules bind every change; the surface contract lives in the specifications repository, and the JavaScript server package is the reference implementation this package mirrors.

## Descriptive names

Use full domain words: `channelReferenceFailures`, `appendTokenPayload`, `signingSecret`. No abbreviations, and no single-letter names outside tight loops. A name says what the thing is; a comment exists only to state a constraint the code cannot show.

## Simplicity over abstraction

Solve the problem in front of you with the simplest structure that stays readable. No registries, factories, event frameworks, dependency-injection containers or wrapper layers. Prefer a function over a type, and a method on an existing type over a new type.

## Maintainability

- One package at the module root; small files with one responsibility.
- Every fixed value lives in `constants.go`, as an unexported constant in Go's MixedCaps.
- Delete code in the same change that obsoletes it.
- Every exported identifier traces to a requirement or a recorded decision in the specifications repository. The surface is pinned in one list in `package_test.go`.
- Errors are `celeris.ErrConfiguration`, naming every failed field and the rule it broke; never the value, the secret or a wrapped cause.
- Signing uses only the standard library (`crypto/hmac`, `crypto/sha512`, `encoding/base64`, `encoding/hex`).
- Golden vectors are produced by independent implementations, never by the code under test.
- Before completion, review the full diff for anything deletable without weakening behaviour or tests.

## Layout

Leave one blank line after every block (`if`, `for`, `switch`, `func` literal spanning lines) before the next statement, except before `else` or at the end of an enclosing block. Separate a declaration group from the block that uses it.

`gofmt` owns everything else about layout; `make check` must pass.
