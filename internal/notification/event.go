package notification

import (
	"fmt"
	"strings"

	"github.com/777genius/claude-notifications/internal/analyzer"
)

// Event is the transport-agnostic notification payload.
// Built once by hooks, consumed independently by Desktop, Webhook, and OSC transports.
type Event struct {
	Status      analyzer.Status
	SessionID   string
	CWD         string
	SessionName string
	GitBranch   string
	Folder      string
	Title       string
	Body        string
}

// RenderEnhancedMessage builds the legacy "[session|branch folder] body" string.
// This is the SINGLE source of truth for the transitional string format.
// Used by hooks to feed webhook's existing SendAsync(status, message, sessionID).
// Will be removed when webhook migrates to Event.
func RenderEnhancedMessage(evt Event) string {
	prefix := evt.SessionName
	if evt.GitBranch != "" && evt.Folder != "" {
		prefix = fmt.Sprintf("%s|%s %s", evt.SessionName, evt.GitBranch, evt.Folder)
	} else if evt.Folder != "" {
		prefix = strings.TrimSpace(evt.SessionName + " " + evt.Folder)
	}
	if prefix == "" {
		return evt.Body
	}
	return fmt.Sprintf("[%s] %s", prefix, evt.Body)
}
