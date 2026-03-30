//go:build windows

package notifier

import "testing"

func TestTitleMatchesFolder(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		folderName string
		want       bool
	}{
		// Windows Terminal with em dash (U+2014)
		{
			name:       "windows terminal em dash",
			title:      "file.go \u2014 my-project \u2014 Windows Terminal",
			folderName: "my-project",
			want:       true,
		},
		// VS Code with em dash
		{
			name:       "vscode em dash",
			title:      "main.go \u2014 my-project \u2014 Visual Studio Code",
			folderName: "my-project",
			want:       true,
		},
		// JetBrains with en dash (U+2013)
		{
			name:       "jetbrains en dash",
			title:      "file.go \u2013 my-project \u2013 GoLand",
			folderName: "my-project",
			want:       true,
		},
		// Regular hyphen
		{
			name:       "hyphen separator",
			title:      "file.go - my-project - Terminal",
			folderName: "my-project",
			want:       true,
		},
		// Git Bash MINGW64 format
		{
			name:       "git bash mingw64",
			title:      "MINGW64:/c/Projects/my-project",
			folderName: "my-project",
			want:       true,
		},
		// Git Bash MINGW32 format
		{
			name:       "git bash mingw32",
			title:      "MINGW32:/c/Users/test/my-project",
			folderName: "my-project",
			want:       true,
		},
		// PowerShell format
		{
			name:       "powershell",
			title:      "PS C:\\Projects\\my-project>",
			folderName: "my-project",
			want:       true,
		},
		// Substring should NOT match (avoid "my-project-v2" matching "my-project")
		{
			name:       "no substring match",
			title:      "file.go \u2014 my-project-v2 \u2014 Windows Terminal",
			folderName: "my-project",
			want:       false,
		},
		// Empty title
		{
			name:       "empty title",
			title:      "",
			folderName: "my-project",
			want:       false,
		},
		// Empty folder name
		{
			name:       "empty folder name",
			title:      "file.go \u2014 my-project \u2014 Windows Terminal",
			folderName: "",
			want:       false,
		},
		// Exact folder name as title component with spaces
		{
			name:       "folder with spaces trimmed",
			title:      "file.go \u2014  my-project  \u2014 Windows Terminal",
			folderName: "my-project",
			want:       true,
		},
		// No matching component
		{
			name:       "no match",
			title:      "file.go \u2014 other-project \u2014 Windows Terminal",
			folderName: "my-project",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleMatchesFolder(tt.title, tt.folderName)
			if got != tt.want {
				t.Errorf("titleMatchesFolder(%q, %q) = %v, want %v", tt.title, tt.folderName, got, tt.want)
			}
		})
	}
}

func TestIsValidFolderName(t *testing.T) {
	tests := []struct {
		name string
		input string
		want bool
	}{
		{"normal name", "my-project", true},
		{"name with dots", "my.project", true},
		{"single char", "a", true},
		{"empty", "", false},
		{"dot", ".", false},
		{"dotdot", "..", false},
		{"separator backslash", "\\", false},
		{"forward slash", "/", true}, // On Windows, filepath.Separator is \, so / is technically a valid name
		{"hidden dir", ".git", true},
		{"name with spaces", "my project", true},
		{"unicode name", "專案", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidFolderName(tt.input)
			if got != tt.want {
				t.Errorf("isValidFolderName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
