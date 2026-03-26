package osc

import (
	"fmt"
	"io"
	"os"

	"github.com/777genius/claude-notifications/internal/notification"
)

// Writer is the interface for writing to /dev/tty (abstracted for testing).
type Writer interface {
	io.Writer
	io.Closer
}

// Config holds OSC sender configuration.
type Config struct {
	Format string // "auto", "osc777", "osc9", "osc99"
}

// Sender writes OSC notifications to the controlling terminal.
type Sender struct {
	format    Format
	wrapper   Wrapper
	ttyOpener func() (Writer, error)
}

// New creates a Sender with the given configuration.
// Wrapper detection (tmux/screen) is cached at init time.
func New(cfg Config) *Sender {
	return &Sender{
		format:    DetectFormat(cfg.Format),
		wrapper:   DetectWrapper(),
		ttyOpener: openTTY,
	}
}

// SendEvent renders a notification.Event and writes it to /dev/tty.
func (s *Sender) SendEvent(evt notification.Event) error {
	title, body := RenderEvent(evt)
	return s.Send(title, body)
}

// Send sanitizes, encodes, wraps and writes an OSC notification to /dev/tty.
func (s *Sender) Send(title, body string) error {
	title = SanitizeText(title, 256)
	body = SanitizeText(body, 512)

	encoded := s.format.Encode(title, body)
	wrapped := s.wrapper.Wrap(encoded)

	w, err := s.ttyOpener()
	if err != nil {
		return fmt.Errorf("cannot open tty: %w", err)
	}
	defer w.Close()

	return writeAll(w, wrapped)
}

// openTTY opens /dev/tty for writing.
func openTTY() (Writer, error) {
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

// writeAll writes all bytes, handling short writes.
func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return fmt.Errorf("tty write failed: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("tty write failed: %w", io.ErrShortWrite)
		}
		data = data[n:]
	}
	return nil
}
