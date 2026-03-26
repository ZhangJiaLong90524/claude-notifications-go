package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsToastEnabled_NonExistentApp(t *testing.T) {
	// An app that has never been registered should return true (default enabled).
	result := IsToastEnabled("NonExistentTestApp_12345_ShouldNotExist")
	assert.True(t, result, "IsToastEnabled should return true for unknown apps (default enabled)")
}

func TestIsToastEnabled_EmptyAppName(t *testing.T) {
	// Empty app name: Windows won't match any registry key → default enabled
	// Non-Windows stub always returns true
	result := IsToastEnabled("")
	assert.True(t, result, "IsToastEnabled with empty name should return true")
}
