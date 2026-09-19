package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
)

const socketName = "kabat.sock"

// DefaultSocketPath is the Unix socket the GUI listens on and the native host dials.
func DefaultSocketPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			// HOME unset: fall back to the temp dir for the socket path.
			return filepath.Join(os.TempDir(), socketName)
		}
		return filepath.Join(home, "Library", "Application Support", "kabat", socketName)
	default:
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, socketName)
		}
		return filepath.Join(os.TempDir(), fmt.Sprintf("kabat-%d.sock", os.Getpid()))
	}
}

// ListenUnix removes a stale socket and listens with mode 0600.
func ListenUnix(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lis, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		closeErr := lis.Close()
		return nil, errors.Join(err, closeErr)
	}
	return lis, nil
}
