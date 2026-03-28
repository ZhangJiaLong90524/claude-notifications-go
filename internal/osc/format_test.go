package osc

import (
	"bytes"
	"os"
	"testing"
)

func TestOSC777Encode(t *testing.T) {
	f := osc777Format{}
	got := f.Encode("Hello", "World")
	want := []byte("\x1b]777;notify;Hello;World\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("osc777 basic encode:\n got %q\nwant %q", got, want)
	}
}

func TestOSC777SemicolonInTitle(t *testing.T) {
	f := osc777Format{}
	got := f.Encode("A;B", "C")
	want := []byte("\x1b]777;notify;A:B;C\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("osc777 semicolon in title:\n got %q\nwant %q", got, want)
	}
}

func TestOSC777SemicolonInBody(t *testing.T) {
	f := osc777Format{}
	got := f.Encode("T", "a;b;c")
	want := []byte("\x1b]777;notify;T;a;b;c\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("osc777 semicolon in body:\n got %q\nwant %q", got, want)
	}
}

func TestOSC9Encode(t *testing.T) {
	f := osc9Format{}
	got := f.Encode("Done", "OK")
	want := []byte("\x1b]9;Done: OK\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("osc9 basic encode:\n got %q\nwant %q", got, want)
	}
}

func TestOSC9EmptyBody(t *testing.T) {
	f := osc9Format{}
	got := f.Encode("Alert", "")
	want := []byte("\x1b]9;Alert\x07")
	if !bytes.Equal(got, want) {
		t.Errorf("osc9 empty body:\n got %q\nwant %q", got, want)
	}
}

func TestOSC99Encode(t *testing.T) {
	f := osc99Format{}
	got := f.Encode("T", "B")
	want := []byte("\x1b]99;i=1:d=0:p=title;T\x1b\\\x1b]99;i=1:d=1:p=body;B\x1b\\")
	if !bytes.Equal(got, want) {
		t.Errorf("osc99 basic encode:\n got %q\nwant %q", got, want)
	}
}

func TestDetectFormatExplicit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"osc777", "osc777"},
		{"osc9", "osc9"},
		{"osc99", "osc99"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			f := DetectFormat(tt.input)
			if f.Name() != tt.want {
				t.Errorf("DetectFormat(%q).Name() = %q, want %q", tt.input, f.Name(), tt.want)
			}
		})
	}
}

func TestDetectFormatAutoDefault(t *testing.T) {
	clearDetectionEnv(t)

	f := DetectFormat("auto")
	if f.Name() != "osc777" {
		t.Errorf("DetectFormat(\"auto\") with no env = %q, want osc777", f.Name())
	}
}

func TestDetectFormatAutoKitty(t *testing.T) {
	clearDetectionEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "42")

	f := DetectFormat("auto")
	if f.Name() != "osc99" {
		t.Errorf("DetectFormat(\"auto\") with KITTY_WINDOW_ID = %q, want osc99", f.Name())
	}
}

func TestDetectFormatAutoITerm(t *testing.T) {
	clearDetectionEnv(t)
	t.Setenv("TERM_PROGRAM", "iTerm.app")

	f := DetectFormat("auto")
	if f.Name() != "osc9" {
		t.Errorf("DetectFormat(\"auto\") with TERM_PROGRAM=iTerm.app = %q, want osc9", f.Name())
	}
}

func TestDetectFormatAutoGhostty(t *testing.T) {
	clearDetectionEnv(t)
	t.Setenv("TERM_PROGRAM", "ghostty")

	f := DetectFormat("auto")
	if f.Name() != "osc777" {
		t.Errorf("DetectFormat(\"auto\") with TERM_PROGRAM=ghostty = %q, want osc777", f.Name())
	}
}

func TestDetectFormatUnknownFallback(t *testing.T) {
	f := DetectFormat("unknown_format")
	if f.Name() != "osc777" {
		t.Errorf("DetectFormat(\"unknown_format\").Name() = %q, want osc777", f.Name())
	}
}

func TestFormatNames(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{osc777Format{}, "osc777"},
		{osc9Format{}, "osc9"},
		{osc99Format{}, "osc99"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.format.Name(); got != tt.want {
				t.Errorf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// clearDetectionEnv unsets all environment variables used by DetectFormat
// so auto-detection falls through to the default. Uses os.Unsetenv + t.Cleanup
// because t.Setenv("KEY", "") still makes os.LookupEnv return (_, true).
func clearDetectionEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"KITTY_WINDOW_ID", "TERM_PROGRAM", "WEZTERM_PANE", "WT_SESSION"} {
		val, ok := os.LookupEnv(key)
		os.Unsetenv(key)
		t.Cleanup(func() {
			if ok {
				os.Setenv(key, val)
			} else {
				os.Unsetenv(key)
			}
		})
	}
}
