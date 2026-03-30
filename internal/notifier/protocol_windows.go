//go:build windows

// ABOUTME: Windows Protocol Activation handler for click-to-focus support.
// ABOUTME: Registers URI scheme in HKCU and handles toast click activation.
package notifier

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/777genius/claude-notifications/internal/logging"
)

const (
	// URIScheme is the custom protocol scheme registered for click-to-focus activation.
	// Used by main.go and focus binary to detect protocol activation URIs.
	URIScheme = "claude-notifications-go"
	// focusBinaryName is the GUI-subsystem binary that handles protocol activation.
	// Built with -ldflags="-H windowsgui" to avoid console window flash.
	// Must match the output name in Makefile/CI build commands.
	focusBinaryName = "claude-notifications-focus-windows-amd64.exe"
)

// EnsureProtocolRegistered ensures the claude-notifications-go:// URI scheme
// is registered in the Windows Registry (HKCU). This is idempotent — the first
// call writes the registry entries (~5ms), subsequent calls verify and update
// the exe path if needed (~1ms).
//
// Registry structure:
//
//	HKCU\Software\Classes\claude-notifications-go\
//	  (Default) = "URL:Claude Notifications Protocol"
//	  "URL Protocol" = ""
//	  shell\open\command\
//	    (Default) = "\"C:\path\to\claude-notifications.exe\" \"%1\""
func EnsureProtocolRegistered() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Prefer the GUI-subsystem focus binary (no console flash on activation).
	// Falls back to the main binary if the focus binary is not found.
	focusExe := filepath.Join(filepath.Dir(exe), focusBinaryName)
	if _, err := os.Stat(focusExe); err != nil {
		logging.Debug("Focus binary not found at %s, using main binary", focusExe)
		focusExe = exe
	}

	keyPath := `Software\Classes\` + URIScheme

	// Create or open the scheme key
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry CreateKey %s: %w", keyPath, err)
	}
	defer k.Close()

	if err := k.SetStringValue("", "URL:Claude Notifications Protocol"); err != nil {
		return fmt.Errorf("registry set default value: %w", err)
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return fmt.Errorf("registry set URL Protocol: %w", err)
	}

	// Create shell\open\command subkey
	cmdPath := keyPath + `\shell\open\command`
	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, cmdPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry CreateKey %s: %w", cmdPath, err)
	}
	defer cmdKey.Close()

	cmdValue := fmt.Sprintf(`"%s" "%%1"`, focusExe)
	if err := cmdKey.SetStringValue("", cmdValue); err != nil {
		return fmt.Errorf("registry set command: %w", err)
	}

	logging.Debug("Protocol %s:// registered: %s", URIScheme, cmdValue)
	return nil
}

// buildProtocolURI constructs the protocol activation URI.
// hwnd is the terminal window handle captured at notification time; if non-zero,
// the protocol handler uses it directly instead of title-based window search.
//
// Uses semicolons (;) instead of ampersands (&) to separate query parameters.
// This follows the Windows Community Toolkit ToastArguments convention:
// Windows ShellExecute treats & as a shell separator when launching protocol
// handlers, which silently breaks multi-parameter URIs in toast launch attributes.
//
// Example output:
//
//	claude-notifications-go://focus?cwd=%2Fc%2FProjects%2Fmy-project;hwnd=2622762
func buildProtocolURI(cwd string, hwnd uintptr) string {
	// Build query string with semicolons instead of ampersands.
	// Cannot use url.Values.Encode() as it uses &.
	var parts []string
	if cwd != "" {
		parts = append(parts, "cwd="+url.QueryEscape(cwd))
	}
	if hwnd != 0 {
		parts = append(parts, "hwnd="+strconv.FormatUint(uint64(hwnd), 10))
	}
	u := url.URL{
		Scheme:   URIScheme,
		Host:     "focus",
		RawQuery: strings.Join(parts, ";"),
	}
	return u.String()
}

// HandleProtocolActivation is called from main.go when the exe is launched by
// Windows Runtime as a protocol handler (user clicked a Toast notification).
//
// The full URI is passed as os.Args[1]:
//
//	claude-notifications-go://focus?cwd=%2Fc%2FProjects%2Fmy-project;hwnd=2622762
//
// Query parameters use semicolons (;) instead of ampersands (&) as separators,
// following the Windows Community Toolkit ToastArguments convention. This avoids
// Windows ShellExecute treating & as a shell separator.
//
// Strategy: try HWND first (direct, works even when Claude Code overrides the
// terminal title), fall back to folder-name title matching.
func HandleProtocolActivation(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid protocol URI: %w", err)
	}

	if u.Host != "focus" {
		return fmt.Errorf("unsupported protocol action: %s", u.Host)
	}

	// Parse semicolon-separated query parameters (not standard &-separated).
	params := parseSemicolonQuery(u.RawQuery)
	cwd := params["cwd"]
	hwndStr := params["hwnd"]
	logging.Debug("Protocol activation: cwd=%s hwnd=%s", cwd, hwndStr)

	// Strategy 1: Direct HWND focus (captured at notification time)
	if hwndStr != "" {
		if hwndVal, err := strconv.ParseUint(hwndStr, 10, 64); err == nil && hwndVal != 0 {
			h := windows.HWND(hwndVal)
			if isWindowValid(h) {
				logging.Debug("Focusing via captured HWND: 0x%X", hwndVal)
				return focusWindow(h)
			}
			logging.Debug("Captured HWND 0x%X is no longer valid, falling back to title match", hwndVal)
		}
	}

	// No fallback: Claude Code overrides the terminal title with its own
	// status line, so title-based window search would never match.
	// The HWND captured at notification time (Strategy 1) is the only
	// reliable method for click-to-focus.
	return fmt.Errorf("protocol activation: no valid HWND available")
}

// parseSemicolonQuery parses a query string that uses semicolons as separators.
// This follows the Windows Community Toolkit ToastArguments convention where
// semicolons replace ampersands to avoid ShellExecute issues.
func parseSemicolonQuery(rawQuery string) map[string]string {
	params := make(map[string]string)
	for _, part := range strings.Split(rawQuery, ";") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if key, err := url.QueryUnescape(k); err == nil {
			k = key
		}
		if val, err := url.QueryUnescape(v); err == nil {
			v = val
		}
		if k != "" {
			params[k] = v
		}
	}
	return params
}
