package celerisserver

import (
	"strconv"
	"time"
)

// appendTokenPayload writes the token payload exactly as the reference
// signer's JSON.stringify does, key order included, so the same claims sign
// to the same bytes in every SDK. encoding/json cannot: it escapes <, >, &,
// U+2028 and U+2029, which JSON.stringify leaves as they are.
func appendTokenPayload(buffer []byte, timestamp int64, claims Claims) []byte {
	buffer = append(buffer, `{"timestamp":`...)
	buffer = strconv.AppendInt(buffer, timestamp, 10)

	if claims.Reference != "" {
		buffer = append(buffer, `,"reference":`...)
		buffer = appendJSONString(buffer, claims.Reference)
	}

	buffer = append(buffer, `,"channel_references":`...)

	if claims.Channels.kind == all {
		buffer = append(buffer, "null"...)
	} else {
		buffer = append(buffer, '[')

		for index, reference := range claims.Channels.references {
			if index > 0 {
				buffer = append(buffer, ',')
			}

			buffer = appendJSONString(buffer, reference)
		}

		buffer = append(buffer, ']')
	}

	buffer = append(buffer, `,"token_permission":`...)

	if claims.Permissions.kind == all {
		buffer = appendPermission(append(buffer, '{'), claims.Permissions.read, claims.Permissions.write)
		buffer = append(buffer, '}')
	} else {
		buffer = append(buffer, '[')

		for index, segment := range claims.Permissions.segments {
			if index > 0 {
				buffer = append(buffer, ',')
			}

			buffer = append(buffer, `{"segment_id":`...)
			buffer = appendJSONString(buffer, segment.SegmentID)
			buffer = appendPermission(append(buffer, ','), segment.Read, segment.Write)
			buffer = append(buffer, '}')
		}

		buffer = append(buffer, ']')
	}

	buffer = append(buffer, `,"replay":`...)

	switch claims.Replay.kind {
	case backlogReplay:
		buffer = append(buffer, "true"...)
	case lookbackReplay:
		buffer = strconv.AppendInt(buffer, int64(claims.Replay.lookback/time.Millisecond), 10)
	default:
		buffer = append(buffer, "false"...)
	}

	buffer = append(buffer, `,"allow_echo":`...)
	buffer = strconv.AppendBool(buffer, claims.AllowEcho)

	return append(buffer, '}')
} // end function appendTokenPayload

func appendPermission(buffer []byte, read, write bool) []byte {
	buffer = append(buffer, `"read":`...)
	buffer = strconv.AppendBool(buffer, read)
	buffer = append(buffer, `,"write":`...)

	return strconv.AppendBool(buffer, write)
} // end function appendPermission

// appendJSONString quotes text as JSON.stringify does: it escapes the quote,
// the backslash and control characters below U+0020, and writes everything
// else as it is. The text is valid UTF-8, checked before signing.
func appendJSONString(buffer []byte, text string) []byte {
	const hexadecimal = "0123456789abcdef"

	buffer = append(buffer, '"')

	for index := range len(text) {
		character := text[index]

		switch character {
		case '"':
			buffer = append(buffer, '\\', '"')
		case '\\':
			buffer = append(buffer, '\\', '\\')
		case '\b':
			buffer = append(buffer, '\\', 'b')
		case '\f':
			buffer = append(buffer, '\\', 'f')
		case '\n':
			buffer = append(buffer, '\\', 'n')
		case '\r':
			buffer = append(buffer, '\\', 'r')
		case '\t':
			buffer = append(buffer, '\\', 't')
		default:
			if character < 0x20 {
				buffer = append(buffer, '\\', 'u', '0', '0', hexadecimal[character>>4], hexadecimal[character&0xf])
			} else {
				buffer = append(buffer, character)
			}
		}
	}

	return append(buffer, '"')
} // end function appendJSONString
