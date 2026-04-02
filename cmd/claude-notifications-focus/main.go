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
	"runtime"
	"strings"
	"syscall"

	"github.com/go-ole/go-ole"
	"github.com/777genius/claude-notifications/internal/logging"
	"github.com/777genius/claude-notifications/internal/notifier"
)

var procCoInitializeSecurity = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeSecurity")

func init() {
	// Pin main goroutine to OS thread before any COM initialization.
	runtime.LockOSThread()
}

func main() {
	// Initialize COM as MTA before anything else.
	ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	defer ole.CoUninitialize()

	// Set COM security to RPC_C_IMP_LEVEL_IMPERSONATE. When launched via
	// ShellExecute (protocol activation from toast click), the shell may
	// trigger implicit CoInitializeSecurity with RPC_C_IMP_LEVEL_IDENTIFY,
	// which prevents UIA from fully enumerating XAML Islands content
	// (FindAll returns only the Win32 TitleBar). IMPERSONATE allows the
	// UIA provider to service cross-process requests fully.
	// See: https://learn.microsoft.com/en-us/windows/win32/com/setting-processwide-security-with-coinitializesecurity
	const (
		rpcCAuthnLevelDefault  = 0
		rpcCImpLevelImpersonate = 3
		eoacNone               = 0
	)
	hr, _, _ := procCoInitializeSecurity.Call(
		0,                                     // pSecDesc
		uintptr(0xFFFFFFFF),                   // cAuthSvc = -1 (COM negotiates)
		0,                                     // asAuthSvc
		0,                                     // reserved
		uintptr(rpcCAuthnLevelDefault),        // dwAuthnLevel
		uintptr(rpcCImpLevelImpersonate),      // dwImpLevel
		0,                                     // pAuthList
		uintptr(eoacNone),                     // dwCapabilities
		0,                                     // reserved
	)
	// hr == 0: success
	// hr == 0x80010119 (RPC_E_TOO_LATE): already initialized — will use CoSetProxyBlanket
	_ = hr

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
