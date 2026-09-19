package nativehost_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/nativehost"
	"github.com/stretchr/testify/require"
)

func TestIsNativeLaunch(t *testing.T) {
	require.False(t, nativehost.IsNativeLaunch([]string{"kabat"}))
	require.False(t, nativehost.IsNativeLaunch([]string{"kabat", "install-native-host"}))
	require.True(t, nativehost.IsNativeLaunch([]string{"kabat", "chrome-extension://" + nativehost.ChromeExtensionID + "/"}))
	require.True(t, nativehost.IsNativeLaunch([]string{"kabat", "moz-extension://abc/"}))
	require.True(t, nativehost.IsNativeLaunch([]string{"kabat", nativehost.GeckoID}))
	require.True(t, nativehost.IsNativeLaunch([]string{"kabat", "/tmp/" + nativehost.Name + ".json"}))
}

func TestWriteManifests(t *testing.T) {
	chromeDir := t.TempDir()
	firefoxDir := t.TempDir()
	exe := "/Applications/Kabat.app/Contents/MacOS/Kabat"
	require.NoError(t, nativehost.WriteManifests(exe, chromeDir, firefoxDir))

	var chrome map[string]any
	raw, err := os.ReadFile(filepath.Join(chromeDir, nativehost.Name+".json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &chrome))
	require.Equal(t, exe, chrome["path"])
	require.Equal(t, nativehost.Name, chrome["name"])
	require.Equal(t, []any{"chrome-extension://" + nativehost.ChromeExtensionID + "/"}, chrome["allowed_origins"])

	var firefox map[string]any
	raw, err = os.ReadFile(filepath.Join(firefoxDir, nativehost.Name+".json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &firefox))
	require.Equal(t, []any{nativehost.GeckoID}, firefox["allowed_extensions"])
}
