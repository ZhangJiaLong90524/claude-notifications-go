package osc

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SanitizeText removes all non-printable characters from s, keeping only runes
// that satisfy unicode.IsPrint. This is a security-critical allowlist filter
// designed to neutralize terminal escape injection (OSC, CSI, BEL, C0/C1 controls).
//
// Space (U+0020) is explicitly preserved. Tab, newline, carriage return, and all
// other control characters are stripped.
//
// The result is truncated to maxLen runes (not bytes), correctly handling
// multi-byte UTF-8 sequences.
//
// Semicolons and other format-specific delimiters are NOT handled here;
// escaping delimiters is the encoder's responsibility.
func SanitizeText(s string, maxLen int) string {
	if len(s) == 0 {
		return ""
	}

	// Upper-bound allocation: output can't exceed input length in bytes.
	var b strings.Builder
	b.Grow(len(s))

	count := 0
	for i := 0; i < len(s); {
		if count >= maxLen {
			break
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		i += size

		// Strip invalid UTF-8 bytes (includes raw C1 control bytes 0x80-0x9F
		// which are not valid UTF-8 lead bytes and decode as RuneError).
		if r == utf8.RuneError && size == 1 {
			continue
		}

		if !unicode.IsPrint(r) {
			continue
		}

		b.WriteRune(r)
		count++
	}

	return b.String()
}
