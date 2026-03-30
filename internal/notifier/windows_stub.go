//go:build !windows

package notifier

import (
	"fmt"

	"github.com/777genius/claude-notifications/internal/config"
)

// URIScheme is the custom protocol scheme for click-to-focus activation.
// On non-Windows platforms this is only used for compilation; the value
// must match the Windows definition in protocol_windows.go.
const URIScheme = "claude-notifications-go"

// sendWindowsNotification is a stub for non-Windows platforms.
func sendWindowsNotification(title, body, appIcon string, cfg *config.Config, cwd string) error {
	return fmt.Errorf("Windows notifications not available on this platform")
}

// HandleProtocolActivation is a stub for non-Windows platforms.
func HandleProtocolActivation(uri string) error {
	return fmt.Errorf("protocol activation not supported on this platform")
}
