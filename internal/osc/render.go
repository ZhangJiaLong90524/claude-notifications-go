package osc

import "github.com/777genius/claude-notifications/internal/notification"

// RenderEvent produces a title and body suitable for an OSC terminal notification.
func RenderEvent(evt notification.Event) (title, body string) {
	title = evt.Title
	if evt.SessionName != "" {
		title += " [" + evt.SessionName + "]"
	}
	switch {
	case evt.GitBranch != "" && evt.Folder != "":
		body = evt.GitBranch + " \u00b7 " + evt.Folder + " -- " + evt.Body
	case evt.Folder != "":
		body = evt.Folder + " -- " + evt.Body
	default:
		body = evt.Body
	}
	return title, body
}
