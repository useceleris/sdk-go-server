package celerisserver

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	celeris "github.com/useceleris/sdk-go-client"
)

// Every string that is signed must be valid UTF-8, so the signed bytes are
// exactly the text the caller passed (D-003).

func validateSignerOptions(options SignerOptions) error {
	var failures []string

	if rule := colonFreeIdentifierRule(options.ClientID); rule != "" {
		failures = append(failures, failure("ClientID", rule))
	}

	if rule := textRule(options.SigningSecret); rule != "" {
		failures = append(failures, failure("SigningSecret", rule))
	}

	return configurationError("signer options", failures)
}

func validateClaims(claims Claims) error {
	var failures []string

	switch claims.Channels.kind {
	case unset:
		failures = append(failures, failure("Channels", "Required"))
	case restricted:
		failures = append(failures, channelReferenceFailures(claims.Channels.references)...)
	}

	switch claims.Permissions.kind {
	case unset:
		failures = append(failures, failure("Permissions", "Required"))
	case restricted:
		failures = append(failures, segmentFailures(claims.Permissions.segments)...)
	}

	if claims.Reference != "" {
		if rule := colonFreeIdentifierRule(claims.Reference); rule != "" {
			failures = append(failures, failure("Reference", rule))
		}
	}

	if claims.Replay.kind == lookbackReplay {
		lookback := claims.Replay.lookback

		switch {
		case lookback < 0 || lookback > maximumReplayLookback:
			failures = append(failures, failure("Replay", "Lookback must be between 0 and 4294967295 milliseconds"))
		case lookback%time.Millisecond != 0:
			failures = append(failures, failure("Replay", "Lookback must be whole milliseconds"))
		}
	}

	return configurationError("claims", failures)
}

func channelReferenceFailures(references []string) []string {
	if len(references) == 0 {
		return []string{failure("Channels.References", "Must list at least one channel reference")}
	}

	var failures []string
	seen := map[string]bool{}

	for index, reference := range references {
		path := "Channels.References[" + strconv.Itoa(index) + "]"

		if rule := channelReferenceRule(reference); rule != "" {
			failures = append(failures, failure(path, rule))
		} else if seen[reference] {
			failures = append(failures, failure("Channels.References", "Must not repeat a channel reference"))
		}

		seen[reference] = true
	}

	return failures
}

func segmentFailures(segments []SegmentClaim) []string {
	var failures []string
	seen := map[string]bool{}

	for index, segment := range segments {
		path := "Permissions.Segments[" + strconv.Itoa(index) + "].SegmentID"

		if rule := identifierRule(segment.SegmentID); rule != "" {
			failures = append(failures, failure(path, rule))
		} else if seen[segment.SegmentID] {
			failures = append(failures, failure("Permissions.Segments", "Must not repeat a segment ID"))
		}

		seen[segment.SegmentID] = true
	}

	return failures
}

func textRule(text string) string {
	if text == "" {
		return "Must not be empty"
	}

	if !utf8.ValidString(text) {
		return "Must not contain invalid UTF-8"
	}

	return ""
}

func identifierRule(identifier string) string {
	if rule := textRule(identifier); rule != "" {
		return rule
	}

	if strings.ContainsAny(identifier, "\r\n") {
		return "Must not contain CR or LF"
	}

	return ""
}

func colonFreeIdentifierRule(identifier string) string {
	if rule := textRule(identifier); rule != "" {
		return rule
	}

	if strings.ContainsAny(identifier, ":\r\n") {
		return "Must not contain a colon, CR or LF"
	}

	return ""
}

func channelReferenceRule(reference string) string {
	if reference == "" {
		return "Must not be empty"
	}

	if len(reference) > maximumChannelReferenceLength {
		return "Must be at most 255 characters"
	}

	for index := range len(reference) {
		character := reference[index]
		letter := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
		digit := character >= '0' && character <= '9'

		if !letter && !digit && character != '-' && character != '_' {
			return "Must contain only ASCII letters, digits, hyphens (-) or underscores (_)"
		}
	}

	return ""
}

// failure describes one failed field and the rule it broke, never its value.
func failure(path, rule string) string {
	return path + ": " + rule + "."
}

// configurationError names every failure, or returns nil when there is none.
func configurationError(subject string, failures []string) error {
	if failures == nil {
		return nil
	}

	return &celeris.Error{Code: celeris.ErrConfiguration, Message: "Invalid " + subject + ". " + strings.Join(failures, " ")}
}
