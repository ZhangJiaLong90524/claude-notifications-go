package osc

import (
	"bytes"
	"testing"
)

func TestNoopWrapper(t *testing.T) {
	w := noopWrapper{}
	data := []byte("hello world")
	got := w.Wrap(data)
	if !bytes.Equal(got, data) {
		t.Fatalf("noopWrapper.Wrap: got %q, want %q", got, data)
	}
}

func TestTmuxWrapperBasic(t *testing.T) {
	w := tmuxWrapper{}
	// OSC 777 notification sequence: ESC ] 7 7 7 ; ... BEL
	input := []byte("\x1b]777;notify;Hi;There\x07")

	got := w.Wrap(input)

	// Must start with DCS tmux; prefix
	prefix := []byte("\x1bPtmux;")
	if !bytes.HasPrefix(got, prefix) {
		t.Fatalf("missing tmux DCS prefix: got %q", got)
	}

	// Must end with ST (ESC \)
	suffix := []byte("\x1b\\")
	if !bytes.HasSuffix(got, suffix) {
		t.Fatalf("missing ST terminator: got %q", got)
	}

	// The original ESC (0x1b) at position 0 of input must be doubled.
	payload := got[len(prefix) : len(got)-len(suffix)]
	// Count ESC bytes in payload vs input.
	inputEscCount := bytes.Count(input, []byte{0x1b})
	payloadEscCount := bytes.Count(payload, []byte{0x1b})
	if payloadEscCount != inputEscCount*2 {
		t.Fatalf("ESC not doubled: input has %d ESC, payload has %d (want %d)",
			inputEscCount, payloadEscCount, inputEscCount*2)
	}
}

func TestTmuxWrapperMultipleESC(t *testing.T) {
	w := tmuxWrapper{}
	// Three ESC bytes in the input.
	input := []byte{0x1b, 'A', 0x1b, 'B', 0x1b, 'C'}

	got := w.Wrap(input)

	prefix := []byte("\x1bPtmux;")
	suffix := []byte("\x1b\\")
	payload := got[len(prefix) : len(got)-len(suffix)]

	// Each of the 3 ESC bytes must be doubled -> 6 ESC bytes total.
	escCount := bytes.Count(payload, []byte{0x1b})
	if escCount != 6 {
		t.Fatalf("expected 6 ESC in payload, got %d", escCount)
	}

	// Verify the content between doubled ESCs.
	expected := []byte{0x1b, 0x1b, 'A', 0x1b, 0x1b, 'B', 0x1b, 0x1b, 'C'}
	if !bytes.Equal(payload, expected) {
		t.Fatalf("payload mismatch:\ngot  %v\nwant %v", payload, expected)
	}
}

func TestTmuxWrapperNoESC(t *testing.T) {
	w := tmuxWrapper{}
	input := []byte("plain text without escape")

	got := w.Wrap(input)

	prefix := []byte("\x1bPtmux;")
	suffix := []byte("\x1b\\")
	payload := got[len(prefix) : len(got)-len(suffix)]

	if !bytes.Equal(payload, input) {
		t.Fatalf("payload should match input when no ESC present:\ngot  %q\nwant %q", payload, input)
	}
}

func TestScreenWrapperBasic(t *testing.T) {
	w := screenWrapper{}
	input := []byte("\x1b]777;notify;Hi;There\x07")

	got := w.Wrap(input)

	// Must start with DCS prefix (ESC P) but NOT "tmux;".
	prefix := []byte("\x1bP")
	if !bytes.HasPrefix(got, prefix) {
		t.Fatalf("missing screen DCS prefix: got %q", got)
	}

	// Must NOT have tmux; after ESC P.
	if bytes.HasPrefix(got, []byte("\x1bPtmux;")) {
		t.Fatal("screen wrapper should not include tmux; prefix")
	}

	// Must end with ST (ESC \).
	suffix := []byte("\x1b\\")
	if !bytes.HasSuffix(got, suffix) {
		t.Fatalf("missing ST terminator: got %q", got)
	}
}

func TestScreenWrapperESCDoubling(t *testing.T) {
	w := screenWrapper{}
	input := []byte{0x1b, '[', '3', '1', 'm', 'r', 'e', 'd', 0x1b, '[', '0', 'm'}

	got := w.Wrap(input)

	prefix := []byte("\x1bP")
	suffix := []byte("\x1b\\")
	payload := got[len(prefix) : len(got)-len(suffix)]

	inputEscCount := bytes.Count(input, []byte{0x1b})
	payloadEscCount := bytes.Count(payload, []byte{0x1b})
	if payloadEscCount != inputEscCount*2 {
		t.Fatalf("ESC not doubled: input has %d ESC, payload has %d (want %d)",
			inputEscCount, payloadEscCount, inputEscCount*2)
	}
}

func TestParseTmuxVersion(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		want    bool
	}{
		{"tmux 3.4", 3, 3, true},
		{"tmux 3.3", 3, 3, true},
		{"tmux 3.3a", 3, 3, true},
		{"tmux 3.2", 3, 3, false},
		{"tmux 2.9", 3, 3, false},
		{"tmux 4.0", 3, 3, true},
		{"tmux next-3.5", 3, 3, true},
		{"tmux next-3.1", 3, 3, false},
		{"tmux master", 3, 3, true},
		{"garbage", 3, 3, false},
		{"", 3, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseTmuxVersion(tt.version, tt.major, tt.minor)
			if got != tt.want {
				t.Errorf("parseTmuxVersion(%q, %d, %d) = %v, want %v",
					tt.version, tt.major, tt.minor, got, tt.want)
			}
		})
	}
}

func TestDetectWrapperNoEnv(t *testing.T) {
	// Clear both multiplexer env vars.
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")

	w := DetectWrapper()
	if _, ok := w.(noopWrapper); !ok {
		t.Fatalf("expected noopWrapper with no env vars, got %T", w)
	}
}

func TestDetectWrapperSTY(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("STY", "12345.pts-0.hostname")

	w := DetectWrapper()
	if _, ok := w.(screenWrapper); !ok {
		t.Fatalf("expected screenWrapper with STY set, got %T", w)
	}
}

func TestTmuxSocketPath(t *testing.T) {
	tests := []struct {
		name string
		tmux string
		want string
	}{
		{"empty", "", ""},
		{"default", "/private/tmp/tmux-501/default,12345,0", "/private/tmp/tmux-501/default"},
		{"custom socket", "/tmp/my-tmux,999,1", "/tmp/my-tmux"},
		{"no comma", "/tmp/tmux-sock", "/tmp/tmux-sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.tmux)
			if got := tmuxSocketPath(); got != tt.want {
				t.Errorf("tmuxSocketPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
