package osc

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/777genius/claude-notifications/internal/logging"
)

// Wrapper wraps raw OSC/DCS data for delivery through terminal multiplexers.
type Wrapper interface {
	Wrap(data []byte) []byte
}

// --- noopWrapper -----------------------------------------------------------

type noopWrapper struct{}

func (noopWrapper) Wrap(data []byte) []byte { return data }

// --- tmuxWrapper -----------------------------------------------------------

type tmuxWrapper struct{}

func (tmuxWrapper) Wrap(data []byte) []byte {
	// Format: ESC P tmux; <payload-with-doubled-ESC> ESC backslash
	doubled := doubleESC(data)

	buf := make([]byte, 0, len(doubled)+9) // 7 prefix + payload + 2 ST
	buf = append(buf, "\x1bPtmux;"...)
	buf = append(buf, doubled...)
	buf = append(buf, "\x1b\\"...)
	return buf
}

// --- screenWrapper ---------------------------------------------------------

type screenWrapper struct{}

func (screenWrapper) Wrap(data []byte) []byte {
	// Format: ESC P <payload-with-doubled-ESC> ESC backslash
	doubled := doubleESC(data)

	buf := make([]byte, 0, len(doubled)+4) // 2 prefix + payload + 2 ST
	buf = append(buf, "\x1bP"...)
	buf = append(buf, doubled...)
	buf = append(buf, "\x1b\\"...)
	return buf
}

// doubleESC replaces every 0x1b byte with 0x1b 0x1b.
func doubleESC(data []byte) []byte {
	count := bytes.Count(data, []byte{0x1b})
	if count == 0 {
		return data
	}

	out := make([]byte, 0, len(data)+count)
	for _, b := range data {
		out = append(out, b)
		if b == 0x1b {
			out = append(out, 0x1b)
		}
	}
	return out
}

// DetectWrapper probes the runtime environment and returns the appropriate
// DCS passthrough wrapper for the current terminal multiplexer.
func DetectWrapper() Wrapper {
	if os.Getenv("TMUX") != "" {
		return detectTmuxWrapper()
	}
	if os.Getenv("STY") != "" {
		return screenWrapper{}
	}
	return noopWrapper{}
}

// detectTmuxWrapper decides whether the running tmux needs a DCS wrapper.
func detectTmuxWrapper() Wrapper {
	enabled, versionKnown := checkTmuxPassthrough()
	if enabled {
		return tmuxWrapper{}
	}
	if versionKnown {
		// Passthrough is explicitly disabled in a tmux that supports the option.
		logging.Warn("tmux passthrough is disabled; OSC notifications will not reach the outer terminal. " +
			"Run: tmux set -g allow-passthrough on")
		return noopWrapper{}
	}
	// Old tmux (< 3.3) or tmux command failed -- best effort: old tmux had
	// passthrough on by default.
	return tmuxWrapper{}
}

const tmuxCmdTimeout = 500 * time.Millisecond

// tmuxPath returns the resolved path to the tmux binary.
// Duplicated from internal/notifier/tmux.go to avoid import cycle.
func tmuxPath() string {
	if path, err := exec.LookPath("tmux"); err == nil {
		return path
	}
	return "tmux"
}

// tmuxSocketPath extracts the socket path from the $TMUX env var.
// Format: "/private/tmp/tmux-501/default,12345,0" -> "/private/tmp/tmux-501/default"
// Duplicated from internal/notifier/tmux.go to avoid import cycle.
func tmuxSocketPath() string {
	tmux := os.Getenv("TMUX")
	if tmux == "" {
		return ""
	}
	if i := strings.IndexByte(tmux, ','); i > 0 {
		return tmux[:i]
	}
	return tmux
}

// tmuxCmd builds an exec.Cmd that targets the correct tmux server
// (resolved binary path + socket path from $TMUX).
func tmuxCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmdArgs := make([]string, 0, len(args)+2)
	if sp := tmuxSocketPath(); sp != "" {
		cmdArgs = append(cmdArgs, "-S", sp)
	}
	cmdArgs = append(cmdArgs, args...)
	return exec.CommandContext(ctx, tmuxPath(), cmdArgs...)
}

// checkTmuxPassthrough queries tmux for the allow-passthrough option.
// Returns (passthroughEnabled, versionKnown).
func checkTmuxPassthrough() (bool, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCmdTimeout)
	defer cancel()

	out, err := tmuxCmd(ctx, "show-options", "-gqv", "allow-passthrough").Output()
	if err != nil {
		// show-options failed -- could be old tmux or no tmux server.
		// Check version: if >= 3.3, the option should exist but something
		// else went wrong; if < 3.3, old tmux had passthrough on by default.
		return false, isTmux33OrLater()
	}

	val := strings.TrimSpace(string(out))
	switch val {
	case "on", "all":
		return true, true
	default:
		// "off", empty, or anything else -> not enabled but the option exists.
		return false, true
	}
}

// isTmux33OrLater checks whether the tmux binary is version >= 3.3.
func isTmux33OrLater() bool {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCmdTimeout)
	defer cancel()

	out, err := tmuxCmd(ctx, "-V").Output()
	if err != nil {
		return false
	}
	return parseTmuxVersion(string(out), 3, 3)
}

// parseTmuxVersion reports whether versionStr describes tmux >= major.minor.
// Accepted formats: "tmux 3.4", "tmux 3.3a", "tmux next-3.5", "tmux master".
func parseTmuxVersion(versionStr string, major, minor int) bool {
	s := strings.TrimSpace(versionStr)

	// Strip "tmux " prefix.
	const prefix = "tmux "
	if !strings.HasPrefix(s, prefix) {
		return false
	}
	s = s[len(prefix):]

	if s == "master" {
		return true
	}

	// Handle "next-X.Y" format.
	s = strings.TrimPrefix(s, "next-")

	// Parse "X.Y" or "X.Ya" (trailing letter suffix).
	dotIdx := strings.IndexByte(s, '.')
	if dotIdx < 0 {
		return false
	}

	majorPart := s[:dotIdx]
	minorPart := s[dotIdx+1:]

	// Strip trailing non-digit suffix (e.g. "3a" -> "3").
	minorDigits := minorPart
	for i, c := range minorPart {
		if c < '0' || c > '9' {
			minorDigits = minorPart[:i]
			break
		}
	}
	if minorDigits == "" {
		return false
	}

	maj := parseDigits(majorPart)
	min := parseDigits(minorDigits)
	if maj < 0 || min < 0 {
		return false
	}

	if maj != major {
		return maj > major
	}
	return min >= minor
}

// parseDigits is a tiny int parser that avoids strconv for a handful of digits.
// Returns -1 on empty string or overflow-risk inputs (> 10 digits).
func parseDigits(s string) int {
	if len(s) == 0 || len(s) > 10 {
		return -1
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}
