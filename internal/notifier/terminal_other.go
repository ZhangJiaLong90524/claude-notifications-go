//go:build !darwin && !linux && !windows

package notifier

import (
	"fmt"

	"github.com/777genius/claude-notifications/internal/config"
)

// GetTerminalBundleID returns empty string on non-macOS platforms
// as terminal bundle IDs are a macOS-specific concept.
func GetTerminalBundleID(configOverride string) string {
	return ""
}

// GetTerminalNotifierPath returns an error on non-macOS platforms
// as terminal-notifier is macOS-only.
func GetTerminalNotifierPath() (string, error) {
	return "", fmt.Errorf("terminal-notifier is only available on macOS")
}

// IsTerminalNotifierAvailable returns false on non-macOS platforms.
func IsTerminalNotifierAvailable() bool {
	return false
}

// EnsureClaudeNotificationsApp is a no-op on non-macOS platforms.
func EnsureClaudeNotificationsApp() error {
	return nil
}

// sendLinuxNotification is a stub for non-Linux platforms.
func sendLinuxNotification(title, body, appIcon string, cfg *config.Config, cwd string) error {
	return fmt.Errorf("Linux notifications not available on this platform")
}

// IsDaemonAvailable returns false on non-Linux platforms.
func IsDaemonAvailable() bool {
	return false
}

// StartDaemon is a no-op on non-Linux platforms.
func StartDaemon() bool {
	return false
}

// StopDaemon is a no-op on non-Linux platforms.
func StopDaemon() error {
	return nil
}

