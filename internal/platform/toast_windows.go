//go:build windows

package platform

import "golang.org/x/sys/windows/registry"

const toastSettingsBase = `SOFTWARE\Microsoft\Windows\CurrentVersion\Notifications\Settings`

// IsToastEnabled checks if toast notifications are enabled for the given app
// in Windows Settings. Returns true if enabled or if the registry key does
// not exist (notifications are enabled by default).
func IsToastEnabled(appName string) bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		toastSettingsBase+`\`+appName, registry.QUERY_VALUE)
	if err != nil {
		return true // key not found = default enabled
	}
	defer key.Close()

	val, _, err := key.GetIntegerValue("Enabled")
	if err != nil {
		return true // value not found = default enabled
	}

	return val != 0
}
