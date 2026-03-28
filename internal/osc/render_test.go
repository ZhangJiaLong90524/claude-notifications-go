package osc

import (
	"testing"

	"github.com/777genius/claude-notifications/internal/notification"
)

func TestRenderEvent(t *testing.T) {
	tests := []struct {
		name      string
		evt       notification.Event
		wantTitle string
		wantBody  string
	}{
		{
			name:      "full context",
			evt:       notification.Event{Title: "Completed", SessionName: "abc", GitBranch: "main", Folder: "myproject", Body: "All done"},
			wantTitle: "Completed [abc]",
			wantBody:  "main \u00b7 myproject -- All done",
		},
		{
			name:      "no branch",
			evt:       notification.Event{Title: "Question", SessionName: "xyz", Folder: "app", Body: "What?"},
			wantTitle: "Question [xyz]",
			wantBody:  "app -- What?",
		},
		{
			name:      "no session name",
			evt:       notification.Event{Title: "Done", Folder: "proj", Body: "Finished"},
			wantTitle: "Done",
			wantBody:  "proj -- Finished",
		},
		{
			name:      "body only",
			evt:       notification.Event{Title: "Alert", Body: "Something happened"},
			wantTitle: "Alert",
			wantBody:  "Something happened",
		},
		{
			name:      "empty everything",
			evt:       notification.Event{Title: "Info"},
			wantTitle: "Info",
			wantBody:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTitle, gotBody := RenderEvent(tt.evt)
			if gotTitle != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tt.wantTitle)
			}
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
		})
	}
}
