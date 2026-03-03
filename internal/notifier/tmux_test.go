package notifier

import (
	"os"
	"testing"
)

func TestIsTmuxControlMode_NotInTmux(t *testing.T) {
	old := os.Getenv("TMUX")
	os.Unsetenv("TMUX")
	t.Cleanup(func() {
		if old != "" {
			os.Setenv("TMUX", old)
		}
	})
	if IsTmuxControlMode() {
		t.Error("should be false when not in tmux")
	}
}

func TestGetTmuxPaneTarget_PrefersEnvVar(t *testing.T) {
	old := os.Getenv("TMUX_PANE")
	os.Setenv("TMUX_PANE", "%42")
	t.Cleanup(func() {
		if old != "" {
			os.Setenv("TMUX_PANE", old)
		} else {
			os.Unsetenv("TMUX_PANE")
		}
	})

	target, err := GetTmuxPaneTarget()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "%42" {
		t.Errorf("expected %%42, got %q", target)
	}
}

func TestGetTmuxPaneTarget_FallsBackWithoutEnvVar(t *testing.T) {
	old := os.Getenv("TMUX_PANE")
	os.Unsetenv("TMUX_PANE")
	t.Cleanup(func() {
		if old != "" {
			os.Setenv("TMUX_PANE", old)
		}
	})

	// Without $TMUX_PANE and without a real tmux server, the fallback
	// should fail gracefully.
	_, err := GetTmuxPaneTarget()
	if err == nil {
		t.Error("expected error when TMUX_PANE is unset and no tmux server, got nil")
	}
}

func TestGetTmuxPaneTarget_IgnoresEmptyEnvVar(t *testing.T) {
	old := os.Getenv("TMUX_PANE")
	os.Setenv("TMUX_PANE", "")
	t.Cleanup(func() {
		if old != "" {
			os.Setenv("TMUX_PANE", old)
		} else {
			os.Unsetenv("TMUX_PANE")
		}
	})

	// Empty TMUX_PANE should fall through to the display-message fallback.
	_, err := GetTmuxPaneTarget()
	if err == nil {
		t.Error("expected error when TMUX_PANE is empty and no tmux server, got nil")
	}
}
