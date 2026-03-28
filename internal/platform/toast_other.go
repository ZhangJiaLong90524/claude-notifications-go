//go:build !windows

package platform

// IsToastEnabled always returns true on non-Windows platforms.
func IsToastEnabled(appName string) bool {
	return true
}
