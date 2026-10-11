# Checks this module. Run from the repository root:
#
#   make check   formatting, vet, lint, vulnerabilities, tidiness and the unit suites
#   make live    the acceptance suites against a real Celeris stack, from .env
#
# Releasing is pushing a version tag, after the client version it requires is
# published; nothing here does it.

GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.8.0

.PHONY: check live

check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "Run gofmt -w ."; exit 1; }
	go vet ./...
	go run $(GOLANGCI_LINT) run ./...
	go run $(GOVULNCHECK) ./...
	go mod verify
	go test -race -count=1 -v ./...

live:
	cd live && GOWORK=off go vet ./... && GOWORK=off go test -race -count=1 -timeout 15m ./...
