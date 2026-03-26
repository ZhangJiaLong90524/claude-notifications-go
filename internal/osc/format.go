package osc

import (
	"fmt"
	"os"
	"strings"

	"github.com/777genius/claude-notifications/internal/logging"
)

// Format defines the interface for encoding terminal OSC notification sequences.
type Format interface {
	// Name returns the identifier of this format (e.g. "osc777", "osc9", "osc99").
	Name() string
	// Encode produces the raw OSC escape sequence bytes for the given title and body.
	Encode(title, body string) []byte
}

// osc777Format encodes notifications using OSC 777 (Ghostty, WezTerm, foot, urxvt, Contour).
// Sequence: ESC ] 777 ; notify ; TITLE ; BODY BEL
type osc777Format struct{}

func (osc777Format) Name() string { return "osc777" }

func (osc777Format) Encode(title, body string) []byte {
	// Replace semicolons in title — title sits between the 2nd and 3rd delimiter,
	// so a literal `;` would break terminal parsers.
	safeTitle := strings.ReplaceAll(title, ";", ":")
	return fmt.Appendf(nil, "\x1b]777;notify;%s;%s\x07", safeTitle, body)
}

// osc9Format encodes notifications using OSC 9 (iTerm2, Windows Terminal).
// Sequence: ESC ] 9 ; MESSAGE BEL
// No title/body split — they are concatenated as "TITLE: BODY".
type osc9Format struct{}

func (osc9Format) Name() string { return "osc9" }

func (osc9Format) Encode(title, body string) []byte {
	msg := title
	if body != "" {
		msg = title + ": " + body
	}
	return fmt.Appendf(nil, "\x1b]9;%s\x07", msg)
}

// osc99Format encodes notifications using OSC 99 (Kitty).
// Two sequences terminated by ST (ESC \):
//
//	ESC ] 99 ; i=1:d=0:p=title ; TITLE ST
//	ESC ] 99 ; i=1:d=1:p=body  ; BODY  ST
type osc99Format struct{}

func (osc99Format) Name() string { return "osc99" }

func (osc99Format) Encode(title, body string) []byte {
	titleSeq := fmt.Sprintf("\x1b]99;i=1:d=0:p=title;%s\x1b\\", title)
	bodySeq := fmt.Sprintf("\x1b]99;i=1:d=1:p=body;%s\x1b\\", body)
	return []byte(titleSeq + bodySeq)
}

// DetectFormat returns the Format matching formatName.
//
// If formatName is empty or "auto", the format is detected from environment
// variables in the following priority order:
//
//  1. KITTY_WINDOW_ID set          -> osc99
//  2. TERM_PROGRAM == "iTerm.app"  -> osc9
//  3. WEZTERM_PANE set             -> osc777
//  4. TERM_PROGRAM == "ghostty"    -> osc777
//  5. TERM_PROGRAM == "foot"       -> osc777
//  6. WT_SESSION set               -> osc9
//  7. default                      -> osc777
func DetectFormat(formatName string) Format {
	switch formatName {
	case "osc777":
		return osc777Format{}
	case "osc9":
		return osc9Format{}
	case "osc99":
		return osc99Format{}
	case "", "auto":
		return detectFromEnv()
	default:
		logging.Warn("unknown OSC format %q, falling back to osc777", formatName)
		return osc777Format{}
	}
}

func detectFromEnv() Format {
	if _, ok := os.LookupEnv("KITTY_WINDOW_ID"); ok {
		return osc99Format{}
	}

	termProgram := os.Getenv("TERM_PROGRAM")

	if termProgram == "iTerm.app" {
		return osc9Format{}
	}
	if _, ok := os.LookupEnv("WEZTERM_PANE"); ok {
		return osc777Format{}
	}
	if termProgram == "ghostty" {
		return osc777Format{}
	}
	if termProgram == "foot" {
		return osc777Format{}
	}
	if _, ok := os.LookupEnv("WT_SESSION"); ok {
		return osc9Format{}
	}

	return osc777Format{}
}
