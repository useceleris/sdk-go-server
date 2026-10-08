package celerisserver

import (
	"encoding/json"
	"testing"
	"unicode/utf8"
)

// FuzzAppendJSONString checks that every valid string quotes to JSON that
// decodes back to it, escaping nothing but the quote, the backslash and
// control characters, as JSON.stringify does.
func FuzzAppendJSONString(f *testing.F) {
	for _, seed := range []string{"", "plain", `"\`, "\x00\x1f\x7f", "\u2028\u2029", "</script>&", "😀", "\ufeff"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			return
		}

		quoted := appendJSONString(nil, text)
		var decoded string

		if err := json.Unmarshal(quoted, &decoded); err != nil || decoded != text {
			t.Fatalf("%q quoted as %s, decoded %q, %v", text, quoted, decoded, err)
		}

		escapes := 0

		for _, character := range []byte(text) {
			if character < 0x20 || character == '"' || character == '\\' {
				escapes++
			}
		}

		// Each escape adds at least one byte; nothing else may grow.
		if escapes == 0 && len(quoted) != len(text)+2 {
			t.Fatalf("%q quoted as %s", text, quoted)
		}
	})
} // end function FuzzAppendJSONString
