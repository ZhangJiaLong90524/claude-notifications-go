package notifier

import (
	"fmt"
	"os"
	"path/filepath"
)

const iTerm2SessionIDEnv = "ITERM_SESSION_ID"

// buildIterm2FocusScript prefers iTerm2's exact session reveal URL when the
// current shell exported ITERM_SESSION_ID. This targets the precise tab/pane
// via the iTerm2 Python API helper. If the helper is unavailable or the exact
// session can no longer be resolved, it falls back to the generic focus-window
// path when cwd is available.
func buildIterm2FocusScript(cwd string) string {
	sessionID := os.Getenv(iTerm2SessionIDEnv)
	pythonPath, scriptPath, ok := getiTerm2PythonEnv()
	if ok && (sessionID != "" || isUsableFocusCWD(cwd)) {
		helperCmd := fmt.Sprintf("%s %s",
			shellQuote(pythonPath),
			shellQuote(scriptPath),
		)
		if sessionID != "" {
			helperCmd += " --termid " + shellQuote(sessionID)
		}
		if isUsableFocusCWD(cwd) {
			helperCmd += " --cwd " + shellQuote(cwd)
		}

		if !isUsableFocusCWD(cwd) {
			return helperCmd
		}

		fallbackCmd := buildBinaryFocusScript(iTerm2BundleID, cwd)
		if fallbackCmd == "" {
			return helperCmd
		}

		// If the exact session helper fails, preserve the previous window-level
		// fallback instead of leaving the click action as a no-op.
		return fmt.Sprintf("%s >/dev/null 2>&1 || %s", helperCmd, fallbackCmd)
	}

	if !isUsableFocusCWD(cwd) {
		return ""
	}
	return buildBinaryFocusScript(iTerm2BundleID, cwd)
}

func isUsableFocusCWD(cwd string) bool {
	if cwd == "" {
		return false
	}
	folderName := filepath.Base(cwd)
	return folderName != "" && folderName != "." && folderName != string(filepath.Separator)
}
