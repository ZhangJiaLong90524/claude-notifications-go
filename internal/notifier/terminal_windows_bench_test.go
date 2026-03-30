//go:build windows

// Benchmark tests for the Windows toast notification pipeline.
// Separated from unit tests because they send real Windows notifications
// and perform registry writes — not suitable for headless CI.
package notifier

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/logging"
)

// initBenchLogger initializes the logger to a fixed temp directory so
// benchmark output is captured. Returns the log file path.
// Uses a fixed path (not t.TempDir) to avoid cleanup issues with open file handles.
// Note: logging.InitLogger uses sync.Once, so only the first call per binary
// actually initializes; subsequent calls are no-ops.
func initBenchLogger(tb testing.TB) string {
	tb.Helper()
	dir := filepath.Join(os.TempDir(), "claude-notifications-bench")
	os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "notification-debug.log")
	os.WriteFile(logPath, nil, 0644)
	t, ok := tb.(*testing.T)
	if ok {
		t.Setenv("CLAUDE_PLUGIN_ROOT", dir)
	} else {
		os.Setenv("CLAUDE_PLUGIN_ROOT", dir)
	}
	logging.InitLogger(dir)
	tb.Cleanup(func() { logging.Close() })
	return logPath
}

// BenchmarkSendWindowsNotification measures the WinRT COM toast push latency
// without click-to-focus. This benchmark verifies that direct COM push (~5ms)
// is significantly faster than the PowerShell fallback (~300ms).
//
// Run: go test -run=^$ -bench=BenchmarkSendWindowsNotification -benchtime=5x -v ./internal/notifier/
func BenchmarkSendWindowsNotification(b *testing.B) {
	initBenchLogger(b)

	cfg := &config.Config{}
	cfg.Notifications.Desktop.ClickToFocus = false
	cfg.Debug.Benchmark = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sendWindowsNotification(
			fmt.Sprintf("Benchmark %d", i),
			"Measuring COM push latency",
			"",
			cfg,
			"",
		)
		if err != nil {
			b.Fatalf("sendWindowsNotification failed: %v", err)
		}
	}
}

// BenchmarkSendWindowsNotificationWithClickToFocus measures the full pipeline
// including HWND discovery via process tree + AttachConsole.
func BenchmarkSendWindowsNotificationWithClickToFocus(b *testing.B) {
	initBenchLogger(b)

	cfg := &config.Config{}
	cfg.Notifications.Desktop.ClickToFocus = true
	cfg.Debug.Benchmark = true

	cwd, _ := os.Getwd()
	if cwd == "" {
		cwd = os.TempDir()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := sendWindowsNotification(
			fmt.Sprintf("Benchmark CTF %d", i),
			"Measuring full click-to-focus pipeline",
			"",
			cfg,
			cwd,
		)
		if err != nil {
			b.Fatalf("sendWindowsNotification failed: %v", err)
		}
	}
}

// TestWindowsNotificationLatencyBreakdown runs a single notification with
// fine-grained timing output. Not a Go benchmark — a diagnostic test that
// prints human-readable timing data.
//
// Run: go test -run=TestWindowsNotificationLatencyBreakdown -v ./internal/notifier/
func TestWindowsNotificationLatencyBreakdown(t *testing.T) {
	logPath := initBenchLogger(t)

	// Test 1: Without click-to-focus (COM push only)
	t.Run("com_push_only", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Debug.Benchmark = true
		cfg.Notifications.Desktop.ClickToFocus = false
		start := time.Now()
		err := sendWindowsNotification("Latency Test", "COM push only", "", cfg, "")
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		t.Logf("Total (no click-to-focus): %dms", elapsed.Milliseconds())
	})

	// Test 2: With click-to-focus (full pipeline)
	t.Run("with_click_to_focus", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.Debug.Benchmark = true
		cfg.Notifications.Desktop.ClickToFocus = true
		cwd, _ := os.Getwd()
		if cwd == "" {
			cwd = os.TempDir()
		}
		start := time.Now()
		err := sendWindowsNotification("Latency Test CTF", "Full pipeline", "", cfg, cwd)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("failed: %v", err)
		}
		t.Logf("Total (with click-to-focus): %dms", elapsed.Milliseconds())
	})

	// Print benchmark log contents
	data, err := os.ReadFile(logPath)
	if err == nil {
		t.Logf("\n=== Benchmark Log ===\n%s", string(data))
	}
}
