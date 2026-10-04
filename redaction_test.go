package celerisserver

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestTheSigningSecretNeverPrints(t *testing.T) {
	options := SignerOptions{ClientID: "synthetic-client", SigningSecret: "synthetic-secret"}
	signer, err := NewSigner(options)

	if err != nil {
		t.Fatal(err)
	}

	// fmt prints unexported fields without calling their methods; a signer
	// keeps its secret where printing cannot reach.
	type holder struct {
		signer *Signer
		value  Signer
	}

	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
		for _, value := range []any{options, &options, *signer, signer, holder{signer: signer, value: *signer}, []*Signer{signer}} {
			if printed := fmt.Sprintf(format, value); strings.Contains(printed, "synthetic-secret") {
				t.Fatalf("%s printed %q", format, printed)
			}
		}
	}

	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("setup", "options", options, "signer", signer)
	slog.New(slog.NewJSONHandler(&logged, nil)).Info("setup", "options", options, "signer", signer)

	if strings.Contains(logged.String(), "synthetic-secret") {
		t.Fatalf("slog printed %q", logged.String())
	}
}
