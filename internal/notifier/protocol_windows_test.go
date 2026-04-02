//go:build windows

package notifier

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestBuildProtocolURI(t *testing.T) {
	tests := []struct {
		name      string
		cwd       string
		hwnd      uintptr
		wantParts []string // substrings that must appear in the URI
		wantAbsent []string // substrings that must NOT appear
	}{
		{
			name: "basic without hwnd",
			cwd:  "/c/Projects/my-project",
			hwnd: 0,
			wantParts: []string{
				"claude-notifications-go://focus?",
				"cwd=",
			},
			wantAbsent: []string{"hwnd="},
		},
		{
			name: "with hwnd",
			cwd:  "/c/Projects/my-project",
			hwnd: 0x28052A,
			wantParts: []string{
				"claude-notifications-go://focus?",
				"cwd=",
				";hwnd=2622762",
			},
			wantAbsent: []string{"&hwnd="},
		},
		{
			name: "path with spaces",
			cwd:  "/c/Projects/my project",
			hwnd: 0,
			wantParts: []string{
				"claude-notifications-go://focus?",
				"cwd=",
			},
		},
		{
			name:      "empty cwd",
			cwd:       "",
			hwnd:      0,
			wantParts: []string{"claude-notifications-go://focus"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProtocolURI(tt.cwd, tt.hwnd, -1)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildProtocolURI(%q, 0x%X) = %q, missing %q", tt.cwd, tt.hwnd, got, part)
				}
			}
			for _, part := range tt.wantAbsent {
				if strings.Contains(got, part) {
					t.Errorf("buildProtocolURI(%q, 0x%X) = %q, should not contain %q", tt.cwd, tt.hwnd, got, part)
				}
			}
		})
	}
}

// TestBuildProtocolURI_SemicolonSeparator verifies that URIs with multiple
// query parameters use semicolons (;) instead of ampersands (&) as separators.
// This follows the Windows Community Toolkit ToastArguments convention:
// Windows ShellExecute treats & as a shell separator, breaking protocol
// activation for multi-parameter URIs in toast launch attributes.
func TestBuildProtocolURI_SemicolonSeparator(t *testing.T) {
	uri := buildProtocolURI("/c/Projects/test", 0x12345, 2)

	// Must use semicolons, not ampersands
	if strings.Contains(uri, "&") {
		t.Errorf("URI contains '&' (should use ';'): %s", uri)
	}
	if !strings.Contains(uri, ";") {
		t.Errorf("URI missing ';' separator: %s", uri)
	}

	// Must be directly XML-safe (no escaping needed)
	testXML := `<toast launch="` + uri + `"></toast>`
	if strings.Contains(testXML, "&") {
		t.Errorf("URI is not XML-safe: %s", testXML)
	}
}

func TestParseSemicolonQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantKeys map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"single param", "cwd=test", map[string]string{"cwd": "test"}},
		{"two params", "cwd=test;hwnd=123", map[string]string{"cwd": "test", "hwnd": "123"}},
		{"trailing semicolon", "cwd=test;", map[string]string{"cwd": "test"}},
		{"leading semicolon", ";cwd=test", map[string]string{"cwd": "test"}},
		{"multiple semicolons", ";;;cwd=test;;;", map[string]string{"cwd": "test"}},
		{"no equals", "keyonly", map[string]string{"keyonly": ""}},
		{"empty value", "key=", map[string]string{"key": ""}},
		{"encoded semicolon in value", "cwd=a%3Bb", map[string]string{"cwd": "a;b"}},
		{"encoded equals in value", "cwd=a%3Db", map[string]string{"cwd": "a=b"}},
		{"url encoded path", "cwd=C%3A%5CUsers%5Ctest", map[string]string{"cwd": `C:\Users\test`}},
		{"duplicate key last wins", "k=1;k=2", map[string]string{"k": "2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSemicolonQuery(tt.input)
			for k, want := range tt.wantKeys {
				if got[k] != want {
					t.Errorf("parseSemicolonQuery(%q)[%q] = %q, want %q", tt.input, k, got[k], want)
				}
			}
			if len(got) != len(tt.wantKeys) {
				t.Errorf("parseSemicolonQuery(%q) has %d keys, want %d", tt.input, len(got), len(tt.wantKeys))
			}
		})
	}
}

func TestHandleProtocolActivation_InvalidURI(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr string
	}{
		{
			name:    "missing hwnd",
			uri:     "claude-notifications-go://focus",
			wantErr: "no HWND in URI",
		},
		{
			name:    "wrong action",
			uri:     "claude-notifications-go://config?cwd=/tmp",
			wantErr: "unsupported protocol action",
		},
		{
			name:    "invalid URI",
			uri:     "://broken",
			wantErr: "invalid protocol URI",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HandleProtocolActivation(tt.uri)
			if err == nil {
				t.Fatalf("HandleProtocolActivation(%q) = nil, want error containing %q", tt.uri, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("HandleProtocolActivation(%q) error = %q, want containing %q", tt.uri, err.Error(), tt.wantErr)
			}
		})
	}
}

// TestBuildProtocolURI_Roundtrip verifies that URIs built by buildProtocolURI
// can be correctly parsed back by HandleProtocolActivation's URL parsing logic.
func TestBuildProtocolURI_Roundtrip(t *testing.T) {
	tests := []struct {
		name string
		cwd  string
		hwnd uintptr
	}{
		{"unix style path", "/c/Projects/my-project", 0},
		{"windows style path", `C:\Projects\my-project`, 0},
		{"path with spaces", "/c/Projects/my project", 0},
		{"path with unicode", "/c/Projects/專案", 0},
		{"path with special chars", "/c/Projects/my-project (v2)", 0},
		{"deep nested path", "/c/Users/test/Documents/Projects/sub/deep/my-project", 0},
		{"with hwnd", "/c/Projects/my-project", 0x28052A},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri := buildProtocolURI(tt.cwd, tt.hwnd, -1)

			// Parse the URI the same way HandleProtocolActivation does
			u, err := url.Parse(uri)
			if err != nil {
				t.Fatalf("url.Parse(%q) failed: %v", uri, err)
			}

			if u.Scheme != URIScheme {
				t.Errorf("scheme = %q, want %q", u.Scheme, URIScheme)
			}
			if u.Host != "focus" {
				t.Errorf("host = %q, want %q", u.Host, "focus")
			}

			// Parse with semicolon separator (same as HandleProtocolActivation)
			params := parseSemicolonQuery(u.RawQuery)
			gotCwd := params["cwd"]
			if gotCwd != tt.cwd {
				t.Errorf("roundtrip cwd = %q, want %q (uri=%q)", gotCwd, tt.cwd, uri)
			}

			// Verify folderName extraction matches
			wantFolder := filepath.Base(tt.cwd)
			gotFolder := filepath.Base(gotCwd)
			if gotFolder != wantFolder {
				t.Errorf("roundtrip folderName = %q, want %q", gotFolder, wantFolder)
			}

			// Verify hwnd roundtrip
			if tt.hwnd != 0 {
				gotHwnd := params["hwnd"]
				wantHwnd := strconv.FormatUint(uint64(tt.hwnd), 10)
				if gotHwnd != wantHwnd {
					t.Errorf("roundtrip hwnd = %q, want %q", gotHwnd, wantHwnd)
				}
			}
		})
	}
}

// TestEnsureProtocolRegistered_Integration writes to the real Windows Registry,
// verifies the entries, then cleans up.
func TestEnsureProtocolRegistered_Integration(t *testing.T) {
	// Register
	if err := EnsureProtocolRegistered(); err != nil {
		t.Fatalf("EnsureProtocolRegistered() failed: %v", err)
	}

	// Cleanup: delete the registry key tree when done
	keyPath := `Software\Classes\` + URIScheme
	t.Cleanup(func() {
		// Delete command subkey first (registry requires bottom-up deletion)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell\open\command`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell\open`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
	})

	// Verify scheme key
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("cannot open scheme key: %v", err)
	}
	defer k.Close()

	defaultVal, _, err := k.GetStringValue("")
	if err != nil {
		t.Fatalf("cannot read default value: %v", err)
	}
	if defaultVal != "URL:Claude Notifications Protocol" {
		t.Errorf("default value = %q, want %q", defaultVal, "URL:Claude Notifications Protocol")
	}

	urlProtocol, _, err := k.GetStringValue("URL Protocol")
	if err != nil {
		t.Fatalf("cannot read URL Protocol: %v", err)
	}
	if urlProtocol != "" {
		t.Errorf("URL Protocol = %q, want empty string", urlProtocol)
	}

	// Verify command key
	cmdPath := keyPath + `\shell\open\command`
	cmdKey, err := registry.OpenKey(registry.CURRENT_USER, cmdPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("cannot open command key: %v", err)
	}
	defer cmdKey.Close()

	cmdVal, _, err := cmdKey.GetStringValue("")
	if err != nil {
		t.Fatalf("cannot read command value: %v", err)
	}
	// Command should contain exe path in quotes + "%1"
	if !strings.Contains(cmdVal, `"%1"`) {
		t.Errorf("command value = %q, missing %%1 placeholder", cmdVal)
	}
	if !strings.HasPrefix(cmdVal, `"`) {
		t.Errorf("command value = %q, exe path not quoted", cmdVal)
	}

	// Verify idempotency: second call should not error
	if err := EnsureProtocolRegistered(); err != nil {
		t.Errorf("second EnsureProtocolRegistered() failed: %v", err)
	}
}

// TestEnsureProtocolRegistered_Idempotent verifies that calling
// EnsureProtocolRegistered multiple times produces the same result.
func TestEnsureProtocolRegistered_Idempotent(t *testing.T) {
	keyPath := `Software\Classes\` + URIScheme
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell\open\command`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell\open`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath+`\shell`)
		_ = registry.DeleteKey(registry.CURRENT_USER, keyPath)
	})

	// Call three times
	for i := 0; i < 3; i++ {
		if err := EnsureProtocolRegistered(); err != nil {
			t.Fatalf("call %d: EnsureProtocolRegistered() failed: %v", i+1, err)
		}
	}

	// Verify final state is correct
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("cannot open scheme key after 3 calls: %v", err)
	}
	k.Close()
}
