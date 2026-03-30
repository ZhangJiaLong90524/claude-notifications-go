//go:build windows

// ABOUTME: Windows-specific window finder and foreground focus via Win32 API.
// ABOUTME: Uses process tree traversal to find the terminal HWND and SetForegroundWindow to focus it.
package notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/777genius/claude-notifications/internal/logging"
)

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procAttachConsole        = kernel32.NewProc("AttachConsole")
	procFreeConsole          = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow     = kernel32.NewProc("GetConsoleWindow")
	procEnumWindows          = user32.NewProc("EnumWindows")
	procGetWindowTextW       = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procIsIconic             = user32.NewProc("IsIconic")
	procIsWindowVisible      = user32.NewProc("IsWindowVisible")
	procIsWindow             = user32.NewProc("IsWindow")
	procGetWindowThreadPID   = user32.NewProc("GetWindowThreadProcessId")
	procGetAncestor          = user32.NewProc("GetAncestor")
)

const (
	swRestore   = 9 // SW_RESTORE for ShowWindow
	gaRootOwner = 3 // GA_ROOTOWNER for GetAncestor
)

// Shell process names — used to find the shell PID in the parent chain
// for AttachConsole.
var shellProcessNames = map[string]bool{
	"pwsh.exe":       true,
	"powershell.exe": true,
	"cmd.exe":        true,
	"bash.exe":       true,
	"zsh.exe":        true,
}

// getTerminalHWND resolves the WT window HWND by walking the parent process
// chain to find the outermost shell, then AttachConsole + GetAncestor(GA_ROOTOWNER).
// Works even with multiple WT windows. Not goroutine-safe (process-global console).
func getTerminalHWND() uintptr {
	shellPID := findOutermostShellPID()
	if shellPID == 0 {
		logging.Debug("No shell found in process tree for HWND resolution")
		return 0
	}

	// Detach from any existing console first — a console-subsystem process
	// gets one automatically at startup, and AttachConsole requires that
	// the process has no console.
	procFreeConsole.Call() //nolint:errcheck

	ret, _, _ := procAttachConsole.Call(uintptr(shellPID))
	if ret == 0 {
		logging.Debug("AttachConsole failed for shell PID %d", shellPID)
		return 0
	}
	defer procFreeConsole.Call() //nolint:errcheck

	consoleHwnd, _, _ := procGetConsoleWindow.Call()
	rootHwnd, _, _ := procGetAncestor.Call(consoleHwnd, gaRootOwner)

	// rootHwnd == consoleHwnd means there is no parent WT window above the console
	// (e.g., standalone conhost.exe or legacy terminal without Windows Terminal).
	if rootHwnd == 0 || rootHwnd == consoleHwnd {
		logging.Debug("Could not resolve WT window (console=0x%X root=0x%X)", consoleHwnd, rootHwnd)
		return 0
	}

	logging.Debug("Terminal HWND resolved: shell PID %d → HWND=0x%X", shellPID, rootHwnd)
	return rootHwnd
}

// findOutermostShellPID walks the parent process chain and returns the PID of the
// farthest ancestor shell process (pwsh.exe, cmd.exe, etc.). The farthest
// shell is preferred because inner shells (e.g., bash from hook-wrapper.sh)
// may not have a real console, while the outermost shell (launched by WT)
// has a ConPTY console that maps to the WT window via GetAncestor.
// Not goroutine-safe due to process-global FreeConsole/AttachConsole calls.
func findOutermostShellPID() uint32 {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		logging.Debug("CreateToolhelp32Snapshot failed: %v", err)
		return 0
	}
	defer windows.CloseHandle(snapshot)

	type procInfo struct {
		parentPID uint32
		name      string
	}
	procs := make(map[uint32]procInfo)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return 0
	}
	for {
		exeName := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
		procs[entry.ProcessID] = procInfo{
			parentPID: entry.ParentProcessID,
			name:      exeName,
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	currentPID := uint32(os.Getpid())
	visited := make(map[uint32]bool)
	var bestShellPID uint32
	var bestShellName string

	for pid := currentPID; pid != 0; {
		if visited[pid] {
			break
		}
		visited[pid] = true

		info, ok := procs[pid]
		if !ok {
			break
		}

		if shellProcessNames[info.name] {
			bestShellPID = pid
			bestShellName = info.name
		}

		pid = info.parentPID
	}

	if bestShellPID != 0 {
		logging.Debug("Outermost shell found: %s (PID %d)", bestShellName, bestShellPID)
	}
	return bestShellPID
}

// knownTerminalProcesses are exe names that own terminal windows.
// Used by isWindowValid to guard against HWND reuse by non-terminal processes.
var knownTerminalProcesses = map[string]bool{
	"windowsterminal.exe": true,
	"cmd.exe":             true,
	"powershell.exe":      true,
	"pwsh.exe":            true,
	"mintty.exe":          true,
	"conhost.exe":         true,
	"code.exe":            true,
	"cursor.exe":          true,
}

// isWindowValid checks whether a HWND still refers to a terminal window.
// Beyond IsWindow(), it verifies the owning process is a known terminal to
// guard against HWND reuse: if the original terminal closed and a different
// application reused the HWND, we fall back to title matching instead of
// focusing the wrong window.
func isWindowValid(hwnd windows.HWND) bool {
	ret, _, _ := procIsWindow.Call(uintptr(hwnd))
	if ret == 0 {
		return false
	}

	// Verify the window still belongs to a terminal process
	var pid uint32
	procGetWindowThreadPID.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid))) //nolint:errcheck
	if pid == 0 {
		return false
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return true // can't verify, assume valid
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return true
	}
	for {
		if entry.ProcessID == pid {
			exeName := strings.ToLower(windows.UTF16ToString(entry.ExeFile[:]))
			return knownTerminalProcesses[exeName]
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return false // process not found, HWND likely stale
}

func isValidFolderName(name string) bool {
	return name != "" && name != "." && name != ".." && name != string(filepath.Separator)
}

// FocusAppWindow focuses the window matching cwd on Windows.
// bundleID is ignored on Windows — we match by window title.
func FocusAppWindow(bundleID, cwd string) error {
	folderName := filepath.Base(cwd)
	if !isValidFolderName(folderName) {
		return fmt.Errorf("invalid cwd: %s", cwd)
	}

	hwnd := findWindowByFolderName(folderName)
	if hwnd == 0 {
		return fmt.Errorf("no window found matching folder: %q", folderName)
	}
	return focusWindow(hwnd)
}

func findWindowByFolderName(folderName string) windows.HWND {
	var found windows.HWND

	cb := syscall.NewCallback(func(hwnd windows.HWND, lParam uintptr) uintptr {
		visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if visible == 0 {
			return 1
		}

		title := getWindowText(hwnd)
		if title == "" {
			return 1
		}

		if titleMatchesFolder(title, folderName) {
			found = hwnd
			return 0
		}
		return 1
	})

	procEnumWindows.Call(cb, 0) //nolint:errcheck
	return found
}

func getWindowText(hwnd windows.HWND) string {
	length, _, _ := procGetWindowTextLengthW.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}

	buf := make([]uint16, length+1)
	procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), length+1) //nolint:errcheck
	return windows.UTF16ToString(buf)
}

// titleMatchesFolder checks if a window title contains folderName as a distinct
// component. Matches the same logic as macOS ax_focus_darwin.go:titleMatchesFolder.
//
// Separator styles:
//
//	Windows Terminal: "file.go — my-project — Windows Terminal" (em dash U+2014)
//	VS Code:         "file.go — my-project — Visual Studio Code" (em dash U+2014)
//	JetBrains:       "file.go – my-project – PhpStorm" (en dash U+2013)
//	Others:          "file.go - my-project - Terminal" (hyphen U+002D)
//
// Also handles Git Bash format:
//
//	"MINGW64:/c/Projects/my-project"
func titleMatchesFolder(title, folderName string) bool {
	if folderName == "" {
		return false
	}

	// Try each separator style (same order as macOS)
	separators := []string{" \u2014 ", " \u2013 ", " - "}
	for _, sep := range separators {
		parts := strings.Split(title, sep)
		for _, part := range parts {
			if strings.TrimSpace(part) == folderName {
				return true
			}
		}
	}

	// Git Bash format: "MINGW64:/c/Projects/my-project"
	if strings.HasPrefix(title, "MINGW64:") || strings.HasPrefix(title, "MINGW32:") {
		path := title[strings.Index(title, ":")+1:]
		if base := filepath.Base(filepath.ToSlash(path)); base == folderName {
			return true
		}
	}

	// PowerShell format: "PS C:\Projects\my-project>"
	if strings.HasPrefix(title, "PS ") && strings.HasSuffix(title, ">") {
		path := strings.TrimPrefix(title, "PS ")
		path = strings.TrimSuffix(path, ">")
		if base := filepath.Base(path); base == folderName {
			return true
		}
	}

	return false
}

// focusWindow brings a window to the foreground.
// Protocol Activation grants the handler process foreground rights, so
// SetForegroundWindow succeeds without additional tricks.
func focusWindow(hwnd windows.HWND) error {
	// Restore if minimized
	iconic, _, _ := procIsIconic.Call(uintptr(hwnd))
	if iconic != 0 {
		procShowWindow.Call(uintptr(hwnd), swRestore) //nolint:errcheck
	}

	ret, _, _ := procSetForegroundWindow.Call(uintptr(hwnd))
	if ret == 0 {
		return fmt.Errorf("SetForegroundWindow failed for hwnd 0x%X", hwnd)
	}

	logging.Debug("Window focused successfully: hwnd=0x%X", hwnd)
	return nil
}
