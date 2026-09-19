package nativehost

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dszlachta/kabat/internal/ipc"
)

const (
	// Name is the native messaging host identifier.
	Name = "com.dszlachta.kabat"
	// GeckoID is the Firefox extension ID.
	GeckoID = "kabat@dszlachta"
	// ChromeExtensionID is derived from the public key in extension/manifest.json.
	ChromeExtensionID = "fajibbphpncllhkbnkfegpmfmmomgokj"
)

// IsNativeLaunch reports whether the process was spawned as a native messaging host.
func IsNativeLaunch(args []string) bool {
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "chrome-extension://") || strings.HasPrefix(a, "moz-extension://") {
			return true
		}
		if a == GeckoID {
			return true
		}
		if strings.HasSuffix(a, Name+".json") {
			return true
		}
	}
	return false
}

// Run dials the GUI Unix socket and copies stdin/stdout (native messaging framing is unchanged).
func Run(socketPath string) error {
	log.SetOutput(os.Stderr)
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() {
		// Shutting down; the GUI socket may already be closed.
		_ = conn.Close()
	}()
	return ipc.Relay(conn, os.Stdin, os.Stdout)
}

type chromeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

type firefoxManifest struct {
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Path              string   `json:"path"`
	Type              string   `json:"type"`
	AllowedExtensions []string `json:"allowed_extensions"`
}

// Install writes Chrome and Firefox native-messaging host manifests for this executable.
func Install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return WriteManifests(exe, chromeHostDir(), firefoxHostDir())
}

// WriteManifests writes host manifests into the given directories.
func WriteManifests(exe, chromeDir, firefoxDir string) error {
	file := Name + ".json"
	chrome := chromeManifest{
		Name:           Name,
		Description:    "Kabat wallet bridge",
		Path:           exe,
		Type:           "stdio",
		AllowedOrigins: []string{"chrome-extension://" + ChromeExtensionID + "/"},
	}
	firefox := firefoxManifest{
		Name:              Name,
		Description:       "Kabat wallet bridge",
		Path:              exe,
		Type:              "stdio",
		AllowedExtensions: []string{GeckoID},
	}
	if err := writeJSON(filepath.Join(chromeDir, file), chrome); err != nil {
		return err
	}
	return writeJSON(filepath.Join(firefoxDir, file), firefox)
}

func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func chromeHostDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// HOME unset: write under a relative empty prefix.
		home = ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts")
	default:
		return filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts")
	}
}

func firefoxHostDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// HOME unset: write under a relative empty prefix.
		home = ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts")
	default:
		return filepath.Join(home, ".mozilla", "native-messaging-hosts")
	}
}
