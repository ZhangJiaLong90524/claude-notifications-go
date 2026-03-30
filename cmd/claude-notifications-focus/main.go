// claude-notifications-focus is the protocol activation handler for Windows
// click-to-focus. It is a separate binary built with -H windowsgui (GUI
// subsystem) so that no console window flashes when Windows launches it
// in response to a toast notification click.
//
// The main claude-notifications binary (console subsystem) captures the
// terminal HWND at notification time and embeds it in the protocol URI.
// This binary receives that URI, extracts the HWND, and calls
// SetForegroundWindow to bring the correct terminal window to front.
//
// Build: go build -ldflags="-H windowsgui" -o bin/claude-notifications-focus-windows-amd64.exe ./cmd/claude-notifications-focus
package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
)

func main() {
	if len(os.Args) < 2 || !strings.HasPrefix(os.Args[1], notifier.URIScheme+"://") {
		os.Exit(1) // Silent: GUI-subsystem binary has no console for error output
	}

	pluginRoot := getPluginRoot()
	_, _ = logging.InitLogger(pluginRoot)
	defer logging.Close()

	if err := notifier.HandleProtocolActivation(os.Args[1]); err != nil {
		logging.Warn("focus handler: %v", err)
	}
}

func getPluginRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	// exe is in bin/, plugin root is parent
	return filepath.Dir(filepath.Dir(exe))
}
