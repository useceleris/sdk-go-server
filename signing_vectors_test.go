package celerisserver

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// signingVector is a credential signed by an independent implementation:
// the first five generated with Python's json, base64, hmac and hashlib, the
// escapes vector and testdata/reference-signing-vectors.json with the
// JavaScript reference signer. Every expected payload and signature is copied
// unchanged.
type signingVector struct {
	Name          string        `json:"name"`
	ClientID      string        `json:"clientId"`
	SigningSecret string        `json:"signingSecret"`
	Timestamp     int64         `json:"timestamp"`
	Claims        vectorClaims  `json:"claims"`
	Expected      vectorPayload `json:"expected"`
} // end struct signingVector

type vectorPayload struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
} // end struct vectorPayload

// vectorClaims is the reference's claims shape, read from JSON.
type vectorClaims struct {
	Channels struct {
		Kind       string   `json:"kind"`
		References []string `json:"references"`
	} `json:"channels"`
	Permissions struct {
		Kind     string `json:"kind"`
		Read     bool   `json:"read"`
		Write    bool   `json:"write"`
		Segments []struct {
			SegmentID string `json:"segmentId"`
			Read      bool   `json:"read"`
			Write     bool   `json:"write"`
		} `json:"segments"`
	} `json:"permissions"`
	Reference string          `json:"reference"`
	Replay    json.RawMessage `json:"replay"`
	AllowEcho bool            `json:"allowEcho"`
} // end struct vectorClaims

func (vector vectorClaims) claims(t *testing.T) Claims {
	t.Helper()

	claims := Claims{Reference: vector.Reference, AllowEcho: vector.AllowEcho, Channels: AllChannels()}

	if vector.Channels.Kind == "restricted" {
		claims.Channels = RestrictedChannels(vector.Channels.References...)
	}

	if vector.Permissions.Kind == "all" {
		claims.Permissions = AllSegments(vector.Permissions.Read, vector.Permissions.Write)
	} else {
		segments := []SegmentClaim{}

		for _, segment := range vector.Permissions.Segments {
			segments = append(segments, SegmentClaim{SegmentID: segment.SegmentID, Read: segment.Read, Write: segment.Write})
		}

		claims.Permissions = RestrictedSegments(segments...)
	}

	switch string(vector.Replay) {
	case "", "false":
	case "true":
		claims.Replay = ReplayBacklog()
	default:
		var lookback struct {
			LookbackMS int64 `json:"lookbackMs"`
		}

		if err := json.Unmarshal(vector.Replay, &lookback); err != nil {
			t.Fatal(err)
		}

		claims.Replay = ReplayLookback(time.Duration(lookback.LookbackMS) * time.Millisecond)
	}

	return claims
} // end method claims

func signingVectors(t *testing.T) []signingVector {
	t.Helper()

	var vectors []signingVector

	if err := json.Unmarshal([]byte(sharedSigningVectors), &vectors); err != nil {
		t.Fatal(err)
	}

	generated, err := os.ReadFile("testdata/reference-signing-vectors.json")

	if err != nil {
		t.Fatal(err)
	}

	var reference []signingVector

	if err := json.Unmarshal(generated, &reference); err != nil {
		t.Fatal(err)
	}

	return append(vectors, reference...)
} // end function signingVectors

func TestSignerMatchesIndependentVectors(t *testing.T) {
	vectors := signingVectors(t)

	if len(vectors) != 24 {
		t.Fatalf("%d vectors", len(vectors))
	}

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			signer, err := NewSigner(SignerOptions{
				ClientID:      vector.ClientID,
				SigningSecret: vector.SigningSecret,
				Clock:         func() time.Time { return time.UnixMilli(vector.Timestamp) },
			})

			if err != nil {
				t.Fatal(err)
			}

			credentials, err := signer.Sign(vector.Claims.claims(t))

			if err != nil {
				t.Fatal(err)
			}

			if credentials.Payload != vector.Expected.Payload || credentials.Signature != vector.Expected.Signature {
				t.Fatalf("got %s / %s", credentials.Payload, credentials.Signature)
			}
		})
	}
} // end function TestSignerMatchesIndependentVectors

// The reference SDK's shared vectors, with the escapes vector the Python port
// generated with the reference signer.
const sharedSigningVectors = `[
  {
    "name": "scoped",
    "clientId": "synthetic-client",
    "signingSecret": "synthetic-secret",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "restricted", "references": ["room-1"]},
      "permissions": {"kind": "restricted", "segments": [{"segmentId": "messages", "read": true, "write": false}]}
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwiY2hhbm5lbF9yZWZlcmVuY2VzIjpbInJvb20tMSJdLCJ0b2tlbl9wZXJtaXNzaW9uIjpbeyJzZWdtZW50X2lkIjoibWVzc2FnZXMiLCJyZWFkIjp0cnVlLCJ3cml0ZSI6ZmFsc2V9XSwicmVwbGF5IjpmYWxzZSwiYWxsb3dfZWNobyI6ZmFsc2V9",
      "signature": "c3ludGhldGljLWNsaWVudDphMjU4ZDM5ZGZiNjgyODlkMzE2NjdlMDVjMGRhNzZmMzAyOTFmMmEwMzBmZmFlZDE0NjU1ZjcyZjNjMDhhNDQ5MWNkZjhiYjQ2Yjk3ZmZiNzY1ZjMyOWM0MWJjZDVlM2ZkMzM2YjY0YTY2NTIzMjdkNTdmZDY1ZDg1YjkwMjhjZQ=="
    }
  },
  {
    "name": "all",
    "clientId": "synthetic-client",
    "signingSecret": "synthetic-secret",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "all"},
      "permissions": {"kind": "all", "read": true, "write": true},
      "replay": true,
      "allowEcho": true
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwiY2hhbm5lbF9yZWZlcmVuY2VzIjpudWxsLCJ0b2tlbl9wZXJtaXNzaW9uIjp7InJlYWQiOnRydWUsIndyaXRlIjp0cnVlfSwicmVwbGF5Ijp0cnVlLCJhbGxvd19lY2hvIjp0cnVlfQ==",
      "signature": "c3ludGhldGljLWNsaWVudDphMGFlMDRjM2RlMTc0YjMzNjhhNjdjNTc1NTgzMTBjZGNjMDVkZjhiZWFjNDRlMTk5NDMwODQ4MzBlZTVjNTA1MWY1OGI1ZDA1MDNjZTEzNTQ4ZmJlM2EyNjdkMzQ1Mjk5OGM3YWYzOWU4NTBiZTYxYmJlNjVmNGNkYTNmNGI0Ng=="
    }
  },
  {
    "name": "deny-all",
    "clientId": "synthetic-client",
    "signingSecret": "synthetic-secret",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "restricted", "references": ["room-1"]},
      "permissions": {"kind": "restricted", "segments": []}
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwiY2hhbm5lbF9yZWZlcmVuY2VzIjpbInJvb20tMSJdLCJ0b2tlbl9wZXJtaXNzaW9uIjpbXSwicmVwbGF5IjpmYWxzZSwiYWxsb3dfZWNobyI6ZmFsc2V9",
      "signature": "c3ludGhldGljLWNsaWVudDo0OThhNzY5OTBlZWFiMzY0Y2E1OGU5ZGU0MDc3YzEyNjA5NjU1MmRkYmQ3MjU1YTE1ZDFmMmVmYWZkMjQ0NmQxNjk0MWJmYmQ4N2U5ZTI5NjNkMTEyNjZmZjk1YzI4NmRhNTdiYzU2OTFhNDViMTg4MzNhZTU5MDdlNDM5MGRlMA=="
    }
  },
  {
    "name": "unicode-zero",
    "clientId": "client-雪",
    "signingSecret": "secret-雪",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "all"},
      "permissions": {"kind": "restricted", "segments": [{"segmentId": "雪", "read": false, "write": true}]},
      "reference": "身分",
      "replay": {"lookbackMs": 0}
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwicmVmZXJlbmNlIjoi6Lqr5YiGIiwiY2hhbm5lbF9yZWZlcmVuY2VzIjpudWxsLCJ0b2tlbl9wZXJtaXNzaW9uIjpbeyJzZWdtZW50X2lkIjoi6ZuqIiwicmVhZCI6ZmFsc2UsIndyaXRlIjp0cnVlfV0sInJlcGxheSI6MCwiYWxsb3dfZWNobyI6ZmFsc2V9",
      "signature": "Y2xpZW50Lembqjo4MjVlZTM0NDA0ODc3YTYwYzJmZmJjZjk1NzA2NGRmZGYzOGNjZTE4NWQxM2JiNzRmNzUxOGE2N2UzZTk0Y2JmMDViNjRlMDQ1MDE2MjhiODIzYzViZTQwYmJlM2RmMDA5M2FjZDk2MTQ4YzE5MDk0NzA0MzkwYTlhZjJmZTk4ZA=="
    }
  },
  {
    "name": "replay-max",
    "clientId": "synthetic-client",
    "signingSecret": "synthetic-secret",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "all"},
      "permissions": {"kind": "all", "read": false, "write": false},
      "replay": {"lookbackMs": 4294967295}
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwiY2hhbm5lbF9yZWZlcmVuY2VzIjpudWxsLCJ0b2tlbl9wZXJtaXNzaW9uIjp7InJlYWQiOmZhbHNlLCJ3cml0ZSI6ZmFsc2V9LCJyZXBsYXkiOjQyOTQ5NjcyOTUsImFsbG93X2VjaG8iOmZhbHNlfQ==",
      "signature": "c3ludGhldGljLWNsaWVudDpiMjZmM2Y2MDAzNThmM2U3OGEwZTQ4NDU3OTE0YmI3NGQzYmMxMWRjMDMxZjU0MjNhYzIxNjJhNzQzZTUzYzg4NGU3YTFlNzE2ZTFjYjlkOTUwNTdiNTkwNGNlZjY0YjVhYjBlZjEyNDNkMmJmOWQ1MmRiZGIzYjJlYjE2ZTk4Zg=="
    }
  },
  {
    "name": "escapes",
    "clientId": "synthetic-client",
    "signingSecret": "synthetic-secret",
    "timestamp": 123456789,
    "claims": {
      "channels": {"kind": "all"},
      "permissions": {"kind": "restricted", "segments": [{"segmentId": "s:</script>\b\f", "read": true, "write": false}]},
      "reference": "u\"\\/\t\u0000\u001f\u007f\u0085\u2028\u2029\ufeff😀"
    },
    "expected": {
      "payload": "eyJ0aW1lc3RhbXAiOjEyMzQ1Njc4OSwicmVmZXJlbmNlIjoidVwiXFwvXHRcdTAwMDBcdTAwMWZ/woXigKjigKnvu7/wn5iAIiwiY2hhbm5lbF9yZWZlcmVuY2VzIjpudWxsLCJ0b2tlbl9wZXJtaXNzaW9uIjpbeyJzZWdtZW50X2lkIjoiczo8L3NjcmlwdD5cYlxmIiwicmVhZCI6dHJ1ZSwid3JpdGUiOmZhbHNlfV0sInJlcGxheSI6ZmFsc2UsImFsbG93X2VjaG8iOmZhbHNlfQ==",
      "signature": "c3ludGhldGljLWNsaWVudDozZDA2ZDAyMjRlYTI5ZGM5Yzc5NjE0YTI5MGZhZGQ2MThjN2UyMGI2N2EyYjI4Y2Q0NmZjNTZkZGE2YTM2ODhiMTM3NjIwMDBiOGYwNWJkN2U2MTAwZjkxNTg2YjcwZjhjOTQ2OGI1N2MwYTgxOGM3NmZhYzdjYmFlNDU4YWVmMQ=="
    }
  }
]`
