package notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// getiTerm2PythonEnv returns the absolute paths to the Python interpreter
// inside the iTerm2 venv and the tab-switch helper script.
// Returns ("", "", false) if either is not found.
func getiTerm2PythonEnv() (pythonPath string, scriptPath string, ok bool) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", "", false
	}

	pythonPath = filepath.Join(homeDir, ".claude",
		"claude-notifications-go", "iterm2-venv", "bin", "python3")
	if _, err := os.Stat(pythonPath); err != nil {
		return "", "", false
	}

	pluginRoot := os.Getenv("CLAUDE_PLUGIN_ROOT")
	if pluginRoot == "" {
		return "", "", false
	}
	scriptPath = filepath.Join(pluginRoot, "scripts", "iterm2-select-tab.py")
	if _, err := os.Stat(scriptPath); err != nil {
		return "", "", false
	}

	return pythonPath, scriptPath, true
}

// buildTmuxCCNotifierArgs constructs terminal-notifier arguments for
// tmux control mode (-CC). Instead of tmux select-window (which doesn't
// switch iTerm2 tabs in -CC mode), calls a Python helper that uses the
// iTerm2 Python API with tmuxWindowPane session variable mapping.
func buildTmuxCCNotifierArgs(title, message, paneTarget, bundleID string) ([]string, error) {
	pythonPath, scriptPath, ok := getiTerm2PythonEnv()
	if !ok {
		return nil, fmt.Errorf("iterm2 venv or helper script not found")
	}

	// paneTarget = "%42", Python script expects "42" (without %)
	paneNum := strings.TrimPrefix(paneTarget, "%")
	executeCmd := fmt.Sprintf("'%s' '%s' '%s'", pythonPath, scriptPath, paneNum)

	args := []string{
		"-title", title,
		"-message", message,
		"-activate", bundleID,
		"-execute", executeCmd,
		"-group", fmt.Sprintf("claude-notif-%d", time.Now().UnixNano()),
	}
	return args, nil
}
