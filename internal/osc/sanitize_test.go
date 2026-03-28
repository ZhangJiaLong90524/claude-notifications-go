package osc

import (
	"strings"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "basic text unchanged",
			input:  "Hello, World!",
			maxLen: 100,
			want:   "Hello, World!",
		},
		{
			name:   "ESC stripped",
			input:  "Hello\x1b[31mRed\x1b[0m",
			maxLen: 100,
			want:   "Hello[31mRed[0m",
		},
		{
			name:   "BEL stripped",
			input:  "Bell\x07Ring",
			maxLen: 100,
			want:   "BellRing",
		},
		{
			name:   "NUL stripped",
			input:  "Null\x00Byte",
			maxLen: 100,
			want:   "NullByte",
		},
		{
			name:   "newlines stripped",
			input:  "Line1\nLine2",
			maxLen: 100,
			want:   "Line1Line2",
		},
		{
			name:   "CR stripped",
			input:  "Line1\rLine2",
			maxLen: 100,
			want:   "Line1Line2",
		},
		{
			name:   "tab stripped",
			input:  "Tab\there",
			maxLen: 100,
			want:   "Tabhere",
		},
		{
			name:   "DEL stripped",
			input:  "Del\x7fete",
			maxLen: 100,
			want:   "Delete",
		},
		{
			name:   "C1 control chars stripped",
			input:  "C1\x9cChar",
			maxLen: 100,
			want:   "C1Char",
		},
		{
			name:   "OSC injection neutralized",
			input:  "\x1b]777;inject;Evil\x1b\\",
			maxLen: 100,
			want:   "]777;inject;Evil\\",
		},
		{
			name:   "emoji preserved",
			input:  "Emoji \U0001f389 OK",
			maxLen: 100,
			want:   "Emoji \U0001f389 OK",
		},
		{
			name:   "cyrillic preserved",
			input:  "\u041a\u0438\u0440\u0438\u043b\u043b\u0438\u0446\u0430",
			maxLen: 100,
			want:   "\u041a\u0438\u0440\u0438\u043b\u043b\u0438\u0446\u0430",
		},
		{
			name:   "CJK preserved",
			input:  "\u65e5\u672c\u8a9e",
			maxLen: 100,
			want:   "\u65e5\u672c\u8a9e",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 100,
			want:   "",
		},
		{
			name:   "max length truncation",
			input:  "abcdefghijklmno",
			maxLen: 10,
			want:   "abcdefghij",
		},
		{
			name:   "max length with emoji counts runes not bytes",
			input:  "Hi \U0001f389\U0001f389\U0001f389\U0001f389\U0001f389\U0001f389\U0001f389\U0001f389",
			maxLen: 6,
			want:   "Hi \U0001f389\U0001f389\U0001f389",
		},
		{
			name:   "spaces preserved",
			input:  "a b c",
			maxLen: 100,
			want:   "a b c",
		},
		{
			name:   "mixed control chars and valid text",
			input:  "\x00\x01\x02Hello\x03\x04\x05",
			maxLen: 100,
			want:   "Hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeText(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("SanitizeText(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestSanitizeText_AllPrintableASCII(t *testing.T) {
	// Every printable ASCII character (0x20 through 0x7e) must pass through.
	var b strings.Builder
	for c := byte(0x20); c <= 0x7e; c++ {
		b.WriteByte(c)
	}
	input := b.String()

	got := SanitizeText(input, 1000)
	if got != input {
		t.Errorf("printable ASCII range was modified:\n  got:  %q\n  want: %q", got, input)
	}
}
