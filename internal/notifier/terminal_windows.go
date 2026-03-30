//go:build windows

// ABOUTME: Windows-specific notification handling with click-to-focus support.
// ABOUTME: Uses go-toast XML template with direct WinRT COM push; PowerShell as fallback.
package notifier

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	toast "git.sr.ht/~jackmordaunt/go-toast"
	"git.sr.ht/~jackmordaunt/go-toast/tmpl"
	"golang.org/x/sys/windows/registry"

	"github.com/777genius/claude-notifications/internal/benchmark"
	"github.com/777genius/claude-notifications/internal/config"
	"github.com/777genius/claude-notifications/internal/logging"
)

const toastAppID = "Claude Code Notifications"

// sendWindowsNotification sends a Windows Toast notification with click-to-focus.
//
// Architecture: go-toast's Push() calls CoRegisterClassObject and writes a
// CustomActivator CLSID to the AUMID registry. This causes Windows to route
// ALL toast activations through COM — even activationType="protocol" — which
// breaks protocol activation after the hook process exits.
//
// Instead, we:
//  1. Build XML using go-toast's exported template
//  2. Register AUMID with a stub CLSID (no COM server) for protocol activation
//  3. Push via direct WinRT COM (~5ms); fall back to PowerShell if COM unavailable
//
// This follows Microsoft's recommended "stub CLSID" pattern for unpackaged
// desktop apps using protocol activation.
func sendWindowsNotification(title, body, appIcon string, cfg *config.Config, cwd string) error {
	bench := benchmark.New(cfg.IsBenchmarkEnabled(), logging.Info)
	bench.Start("windows.total")
	defer bench.Elapsed("windows.total")

	clickToFocus := cfg.Notifications.Desktop.ClickToFocus && cwd != ""
	if clickToFocus {
		bench.Start("windows.protocol_register")
		if err := EnsureProtocolRegistered(); err != nil {
			logging.Warn("Failed to register protocol handler, click-to-focus disabled: %v", err)
			clickToFocus = false
		}
		bench.Elapsed("windows.protocol_register")
	}

	// Sanitize CDATA-unsafe sequences. go-toast's XML template wraps Title/Body
	// in <![CDATA[...]]>, and text/template performs no escaping. A literal "]]>"
	// in the content would prematurely close the CDATA section, breaking the XML.
	title = strings.ReplaceAll(title, "]]>", "]] >")
	body = strings.ReplaceAll(body, "]]>", "]] >")

	// Build notification struct for XML rendering
	n := toast.Notification{
		AppID:          toastAppID,
		Title:          title,
		Body:           body,
		Audio:          toast.Silent,
		ActivationType: toast.Foreground,
		Duration:       "short",
	}
	if appIcon != "" {
		n.Icon = appIcon
	}
	if clickToFocus {
		bench.Start("windows.hwnd_discovery")
		hwnd := getTerminalHWND()
		bench.Elapsed("windows.hwnd_discovery")
		// No XML escaping needed: buildProtocolURI uses semicolons (;) as separators,
		// following Windows Community Toolkit convention. Semicolons are safe in XML.
		n.ActivationType = toast.Protocol
		n.ActivationArguments = buildProtocolURI(cwd, hwnd)
		logging.Debug("Windows notification: clickToFocus HWND=0x%X URI=%s", hwnd, n.ActivationArguments)
	}

	// Render XML
	bench.Start("windows.xml_render")
	var xmlBuf bytes.Buffer
	if err := tmpl.XMLTemplate.Execute(&xmlBuf, &n); err != nil {
		return fmt.Errorf("toast XML template failed: %w", err)
	}
	bench.Elapsed("windows.xml_render")

	// Register AUMID with DisplayName (no CustomActivator — that's handled by ensureStubActivator)
	bench.Start("windows.aumid_register")
	if err := ensureAUMID(toastAppID, appIcon); err != nil {
		logging.Warn("AUMID registration failed: %v", err)
	}
	bench.Elapsed("windows.aumid_register")

	// Write stub CLSID — enables protocol activation on toast click.
	// See ensureStubActivator doc for rationale.
	bench.Start("windows.stub_activator")
	if err := ensureStubActivator(toastAppID); err != nil {
		logging.Debug("Stub activator write failed (non-fatal): %v", err)
	}
	bench.Elapsed("windows.stub_activator")

	// Push via direct WinRT COM (~5ms vs ~300ms PowerShell).
	// Falls back to PowerShell if COM fails (e.g., Windows 7, Server Core, Wine).
	bench.Start("windows.push")
	if err := pushToastCOM(xmlBuf.String(), toastAppID); err != nil {
		logging.Warn("COM push failed, falling back to PowerShell: %v", err)
		bench.Elapsed("windows.push")
		bench.Start("windows.powershell_fallback")
		tag := fmt.Sprintf("cn-%d", time.Now().UnixMilli())
		if err := pushToast(xmlBuf.String(), toastAppID, tag); err != nil {
			return fmt.Errorf("toast push failed: %w", err)
		}
		bench.Elapsed("windows.powershell_fallback")
	} else {
		bench.Elapsed("windows.push")
	}

	return nil
}

// ensureAUMID registers the AppUserModelID in the Windows Registry with
// DisplayName only. Does NOT write CustomActivator — that is handled
// separately by ensureStubActivator.
func ensureAUMID(appID, iconPath string) error {
	keyPath := `SOFTWARE\Classes\AppUserModelId\` + appID
	k, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	_ = k.SetStringValue("DisplayName", appID)
	if iconPath != "" {
		_ = k.SetStringValue("IconUri", iconPath)
	}
	return nil
}

// stubCLSID is a fixed GUID used as a stub CustomActivator. It has no
// corresponding COM server — Windows will fail COM activation and fall
// through to protocol activation (the only activation type supported
// with a stub CLSID per Microsoft docs).
const stubCLSID = "{E1A2B3C4-D5F6-4A7B-8C9D-0E1F2A3B4C5D}"

// ensureStubActivator writes a stub CLSID as CustomActivator to the AUMID
// registry. Per Microsoft docs for unpackaged desktop apps, a stub CLSID
// (with no COM server) enables protocol activation on toast click.
// Without any CLSID, Windows may consider the toast non-activatable.
//
// This replaces cleanCustomActivator — instead of deleting the CLSID
// (which prevents activation entirely), we write our own stub that
// has no COM server registered, forcing Windows to use protocol activation.
//
// Uses OpenKey (not CreateKey) because the AUMID key must already exist
// from ensureAUMID. If it doesn't, the stub write is a no-op.
func ensureStubActivator(appID string) error {
	keyPath := `SOFTWARE\Classes\AppUserModelId\` + appID
	k, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open AUMID key: %w", err)
	}
	defer k.Close()
	if err := k.SetStringValue("CustomActivator", stubCLSID); err != nil {
		return fmt.Errorf("set CustomActivator: %w", err)
	}
	return nil
}

// pushToast sends a toast notification via PowerShell. This is the fallback
// path when direct WinRT COM push is unavailable (e.g., Windows 7, Server
// Core, Wine, or COM initialization failure).
//
// The XML is written to a separate temp file and loaded via ReadAllText
// to avoid PowerShell here-string injection (content containing `"@` on a
// new line would break the here-string, and `]]>` would break CDATA).
func pushToast(toastXML, appID, tag string) error {
	// Write XML to temp file (avoids here-string injection)
	xmlFile, err := os.CreateTemp("", "claude-toast-xml-*.xml")
	if err != nil {
		return fmt.Errorf("failed to create XML temp file: %w", err)
	}
	defer func() { _ = os.Remove(xmlFile.Name()) }()
	if _, err := xmlFile.WriteString(toastXML); err != nil {
		return fmt.Errorf("writing XML: %w", err)
	}
	if err := xmlFile.Close(); err != nil {
		return fmt.Errorf("closing XML file: %w", err)
	}

	var tagLine string
	if tag != "" {
		tagLine = fmt.Sprintf("\n$toast.Tag = '%s'\n$toast.Group = 'claude'", tag)
	}

	// Use ReadAllText to load XML from file, avoiding here-string entirely
	xmlPath := strings.ReplaceAll(xmlFile.Name(), `\`, `\\`)
	script := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.UI.Notifications.ToastNotification, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml([System.IO.File]::ReadAllText('%s'))
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml%s
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('%s').Show($toast)
`, xmlPath, tagLine, appID)

	f, err := os.CreateTemp("", "claude-toast-*.ps1")
	if err != nil {
		return fmt.Errorf("creating temp script: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()

	// UTF-8 BOM for PowerShell non-ASCII support
	if _, err := f.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
		return fmt.Errorf("writing BOM: %w", err)
	}
	if _, err := f.WriteString(script); err != nil {
		return fmt.Errorf("writing PS1 script: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing PS1 file: %w", err)
	}

	cmd := exec.Command("PowerShell", "-ExecutionPolicy", "Bypass", "-File", f.Name())
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW — prevents console window entirely
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("PowerShell: %q: %w", string(out), err)
	}

	logging.Debug("Toast sent via PowerShell (tag=%s)", tag)
	return nil
}

// Platform interface stubs — not applicable on Windows but required for compilation.

// GetTerminalBundleID returns empty string on Windows
// as terminal bundle IDs are a macOS-specific concept.
func GetTerminalBundleID(configOverride string) string {
	return ""
}

// GetTerminalNotifierPath returns an error on Windows
// as terminal-notifier is macOS-only.
func GetTerminalNotifierPath() (string, error) {
	return "", fmt.Errorf("terminal-notifier is only available on macOS")
}

// IsTerminalNotifierAvailable returns false on Windows.
func IsTerminalNotifierAvailable() bool {
	return false
}

// EnsureClaudeNotificationsApp is a no-op on Windows.
func EnsureClaudeNotificationsApp() error {
	return nil
}

// sendLinuxNotification is a stub for Windows.
func sendLinuxNotification(title, body, appIcon string, cfg *config.Config, cwd string) error {
	return fmt.Errorf("Linux notifications not available on Windows")
}

// IsDaemonAvailable returns false on Windows.
func IsDaemonAvailable() bool {
	return false
}

// StartDaemon is a no-op on Windows.
func StartDaemon() bool {
	return false
}

// StopDaemon is a no-op on Windows.
func StopDaemon() error {
	return nil
}
