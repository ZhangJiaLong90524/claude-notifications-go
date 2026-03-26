package osc

import (
	"bytes"
	"errors"
	"testing"
)

type mockWriter struct {
	buf      bytes.Buffer
	writeErr error
	closeErr error
}

func (m *mockWriter) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return m.buf.Write(p)
}

func (m *mockWriter) Close() error {
	return m.closeErr
}

func newTestSender(format Format, wrapper Wrapper) (*Sender, *mockWriter) {
	w := &mockWriter{}
	s := &Sender{
		format:    format,
		wrapper:   wrapper,
		ttyOpener: func() (Writer, error) { return w, nil },
	}
	return s, w
}

func TestSender_Send_Basic(t *testing.T) {
	s, w := newTestSender(osc777Format{}, noopWrapper{})

	err := s.Send("Hello", "World")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	got := w.buf.Bytes()
	want := []byte("\x1b]777;notify;Hello;World\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestSender_Send_WithTmuxWrapper(t *testing.T) {
	s, w := newTestSender(osc777Format{}, tmuxWrapper{})

	err := s.Send("T", "B")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	// Verify it's wrapped in DCS passthrough
	got := w.buf.Bytes()
	if !bytes.HasPrefix(got, []byte("\x1bPtmux;")) {
		t.Errorf("output should start with DCS tmux prefix, got %q", got)
	}
	if !bytes.HasSuffix(got, []byte("\x1b\\")) {
		t.Errorf("output should end with ST, got %q", got)
	}
}

func TestSender_Send_Sanitizes(t *testing.T) {
	s, w := newTestSender(osc777Format{}, noopWrapper{})

	err := s.Send("Title\x1b[31m", "Body\x07bell")
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	got := w.buf.String()
	// ESC and BEL in input should be stripped (only the format's own BEL terminator remains)
	if bytes.Count(w.buf.Bytes(), []byte{0x1b}) != 1 {
		t.Errorf("should have exactly 1 ESC (from OSC prefix), got %q", got)
	}
}

func TestSender_Send_TTYOpenFails(t *testing.T) {
	s := &Sender{
		format:    osc777Format{},
		wrapper:   noopWrapper{},
		ttyOpener: func() (Writer, error) { return nil, errors.New("no tty") },
	}

	err := s.Send("T", "B")
	if err == nil {
		t.Error("expected error when TTY unavailable")
	}
}

func TestSender_Send_WriteFails(t *testing.T) {
	w := &mockWriter{writeErr: errors.New("broken pipe")}
	s := &Sender{
		format:    osc777Format{},
		wrapper:   noopWrapper{},
		ttyOpener: func() (Writer, error) { return w, nil },
	}

	err := s.Send("T", "B")
	if err == nil {
		t.Error("expected error when write fails")
	}
}

func TestSender_Send_Truncates(t *testing.T) {
	s, w := newTestSender(osc777Format{}, noopWrapper{})

	// Create a long title of 'A' repeated 300 times
	longTitleBytes := make([]byte, 300)
	for i := range longTitleBytes {
		longTitleBytes[i] = 'A'
	}
	longTitle := string(longTitleBytes)

	err := s.Send(longTitle, "B")
	if err != nil {
		t.Fatalf("Send error: %v", err)
	}

	// Title should be truncated to 256 chars
	got := w.buf.String()
	// Count 'A's in output -- should be exactly 256
	count := 0
	for _, b := range got {
		if b == 'A' {
			count++
		}
	}
	if count != 256 {
		t.Errorf("expected 256 A's in output, got %d", count)
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
func (zeroWriter) Close() error              { return nil }

func TestSender_Send_ZeroWrite(t *testing.T) {
	s := &Sender{
		format:    osc777Format{},
		wrapper:   noopWrapper{},
		ttyOpener: func() (Writer, error) { return &zeroWriter{}, nil },
	}
	err := s.Send("T", "B")
	if err == nil {
		t.Fatal("expected error on zero write")
	}
}

func TestSender_Send_OSC99WithTmuxWrapper(t *testing.T) {
	s, w := newTestSender(osc99Format{}, tmuxWrapper{})
	if err := s.Send("Title", "Body"); err != nil {
		t.Fatal(err)
	}
	got := w.buf.Bytes()
	if !bytes.HasPrefix(got, []byte("\x1bPtmux;")) {
		t.Fatalf("missing tmux wrapper prefix: %q", got)
	}
	if !bytes.HasSuffix(got, []byte("\x1b\\")) {
		t.Fatalf("missing ST terminator: %q", got)
	}
	// OSC 99 uses ST (\x1b\\) as inner terminators, which contain ESC bytes.
	// Inside DCS passthrough, each ESC must be doubled.
	// The raw osc99 payload has 4 ESC bytes (2 per sequence × 2 sequences).
	// After doubling, the inner payload should have 8 ESC bytes.
	// Plus the DCS envelope adds 2 more ESC bytes (prefix + terminator).
	payload := got[len("\x1bPtmux;") : len(got)-len("\x1b\\")]
	escCount := bytes.Count(payload, []byte{0x1b})
	if escCount < 8 {
		t.Fatalf("expected at least 8 doubled ESC bytes in wrapped osc99 payload, got %d in %q", escCount, got)
	}
}
