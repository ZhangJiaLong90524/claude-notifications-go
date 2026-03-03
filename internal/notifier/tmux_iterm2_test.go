package notifier

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// overrideHome sets HOME (Unix) and USERPROFILE (Windows) to dir,
// so that os.UserHomeDir() returns dir on all platforms.
// Returns a cleanup function that restores the original values.
func overrideHome(t *testing.T, dir string) {
	t.Helper()
	oldHome := os.Getenv("HOME")
	oldProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		os.Setenv("USERPROFILE", dir)
	}
	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		if runtime.GOOS == "windows" {
			if oldProfile != "" {
				os.Setenv("USERPROFILE", oldProfile)
			} else {
				os.Unsetenv("USERPROFILE")
			}
		}
	})
}

// setupFakeiTerm2Env creates a temporary directory with a fake iTerm2 venv
// and helper script, overrides HOME and CLAUDE_PLUGIN_ROOT to point there,
// and registers cleanup to restore the original env.
// Returns the temp dir path.
func setupFakeiTerm2Env(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	// Create fake venv with python3 binary
	venvBin := filepath.Join(tmpDir, ".claude", "claude-notifications-go",
		"iterm2-venv", "bin")
	if err := os.MkdirAll(venvBin, 0o755); err != nil {
		t.Fatalf("failed to create venv dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(venvBin, "python3"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake python3: %v", err)
	}

	// Create fake helper script
	scriptsDir := filepath.Join(tmpDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("failed to create scripts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "iterm2-select-tab.py"), []byte(""), 0o644); err != nil {
		t.Fatalf("failed to write fake script: %v", err)
	}

	// Override HOME (+ USERPROFILE on Windows) and CLAUDE_PLUGIN_ROOT
	overrideHome(t, tmpDir)
	oldRoot := os.Getenv("CLAUDE_PLUGIN_ROOT")
	os.Setenv("CLAUDE_PLUGIN_ROOT", tmpDir)
	t.Cleanup(func() {
		if oldRoot != "" {
			os.Setenv("CLAUDE_PLUGIN_ROOT", oldRoot)
		} else {
			os.Unsetenv("CLAUDE_PLUGIN_ROOT")
		}
	})

	return tmpDir
}

// withIsolatedEnv overrides HOME and CLAUDE_PLUGIN_ROOT to a temp dir
// (without creating any venv files) and restores them on cleanup.
func withIsolatedEnv(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)
	oldRoot := os.Getenv("CLAUDE_PLUGIN_ROOT")
	os.Unsetenv("CLAUDE_PLUGIN_ROOT")
	t.Cleanup(func() {
		if oldRoot != "" {
			os.Setenv("CLAUDE_PLUGIN_ROOT", oldRoot)
		} else {
			os.Unsetenv("CLAUDE_PLUGIN_ROOT")
		}
	})
}

func TestGetiTerm2PythonEnv_NoVenv(t *testing.T) {
	withIsolatedEnv(t)
	_, _, ok := getiTerm2PythonEnv()
	if ok {
		t.Error("should return false when venv doesn't exist")
	}
}

func TestBuildTmuxCCNotifierArgs_NoVenv(t *testing.T) {
	withIsolatedEnv(t)
	_, err := buildTmuxCCNotifierArgs("Title", "Msg", "%42", "com.test")
	if err == nil {
		t.Error("expected error when venv not found")
	}
}

func TestBuildTmuxCCNotifierArgs_StripsPanePrefix(t *testing.T) {
	setupFakeiTerm2Env(t)

	args, err := buildTmuxCCNotifierArgs("Title", "Msg", "%42", "com.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	executeCmd := getArgValue(args, "-execute")
	if strings.Contains(executeCmd, "%42") {
		t.Errorf("-execute should strip %% prefix, got: %s", executeCmd)
	}
	if !strings.Contains(executeCmd, "'42'") {
		t.Errorf("-execute should contain '42', got: %s", executeCmd)
	}
}

func TestBuildTmuxCCNotifierArgs_ContainsActivate(t *testing.T) {
	setupFakeiTerm2Env(t)

	args, err := buildTmuxCCNotifierArgs("Title", "Msg", "%42", "com.googlecode.iterm2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsArg(args, "-activate", "com.googlecode.iterm2") {
		t.Error("missing -activate with iTerm2 bundle ID")
	}
	executeCmd := getArgValue(args, "-execute")
	if !strings.Contains(executeCmd, "iterm2-select-tab.py") {
		t.Errorf("-execute should reference iterm2-select-tab.py, got: %s", executeCmd)
	}
}

func TestBuildTmuxCCNotifierArgs_HasGroup(t *testing.T) {
	setupFakeiTerm2Env(t)

	args, err := buildTmuxCCNotifierArgs("Title", "Msg", "%10", "com.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	group := getArgValue(args, "-group")
	if group == "" {
		t.Error("missing -group argument")
	}
	if !strings.HasPrefix(group, "claude-notif-") {
		t.Errorf("-group should have claude-notif- prefix, got: %s", group)
	}
}

func TestBuildTmuxCCNotifierArgs_PaneWithoutPercent(t *testing.T) {
	setupFakeiTerm2Env(t)

	args, err := buildTmuxCCNotifierArgs("Title", "Msg", "42", "com.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	executeCmd := getArgValue(args, "-execute")
	if !strings.Contains(executeCmd, "'42'") {
		t.Errorf("-execute should contain '42', got: %s", executeCmd)
	}
}

func TestBuildTmuxCCNotifierArgs_EmptyPaneTarget(t *testing.T) {
	setupFakeiTerm2Env(t)

	args, err := buildTmuxCCNotifierArgs("Title", "Msg", "", "com.test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not panic, -execute should contain empty pane arg
	executeCmd := getArgValue(args, "-execute")
	if executeCmd == "" {
		t.Error("-execute should not be empty")
	}
}

func TestGetiTerm2PythonEnv_MissingPluginRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Create fake venv but do NOT set CLAUDE_PLUGIN_ROOT
	venvBin := filepath.Join(tmpDir, ".claude", "claude-notifications-go",
		"iterm2-venv", "bin")
	if err := os.MkdirAll(venvBin, 0o755); err != nil {
		t.Fatalf("failed to create venv dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(venvBin, "python3"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake python3: %v", err)
	}

	overrideHome(t, tmpDir)
	oldRoot := os.Getenv("CLAUDE_PLUGIN_ROOT")
	os.Unsetenv("CLAUDE_PLUGIN_ROOT")
	t.Cleanup(func() {
		if oldRoot != "" {
			os.Setenv("CLAUDE_PLUGIN_ROOT", oldRoot)
		} else {
			os.Unsetenv("CLAUDE_PLUGIN_ROOT")
		}
	})

	_, _, ok := getiTerm2PythonEnv()
	if ok {
		t.Error("should return false when CLAUDE_PLUGIN_ROOT is unset")
	}
}
