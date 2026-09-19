package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const fileName = "config.json"

// MCPListenAddr is the loopback address of the MCP HTTP server.
const MCPListenAddr = "127.0.0.1:19375"

// MCPURL is the streamable HTTP endpoint agents connect to.
func MCPURL() string {
	return "http://" + MCPListenAddr
}

var mu sync.Mutex

// Wallet is persisted display data for one account.
type Wallet struct {
	Name string `json:"name,omitempty"`
}

// Contract is persisted data for one on-chain address.
type Contract struct {
	ABI string `json:"abi,omitempty"`
}

// Window is the last main-window size.
type Window struct {
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// File is the on-disk JSON config.
type File struct {
	Window    *Window             `json:"window,omitempty"`
	AnvilURL  string              `json:"anvilUrl,omitempty"`
	MCP       bool                `json:"mcp,omitempty"`
	Wallets   map[string]Wallet   `json:"wallets,omitempty"`
	Contracts map[string]Contract `json:"contracts,omitempty"`
}

// DefaultPath is the user-level config.json for this machine.
func DefaultPath() string {
	return filepath.Join(dir(), fileName)
}

func dir() string {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return os.TempDir()
		}
		return filepath.Join(home, "Library", "Application Support", "kabat")
	default:
		if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
			return filepath.Join(base, "kabat")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return os.TempDir()
		}
		return filepath.Join(home, ".config", "kabat")
	}
}

// Load reads path. A missing file is an empty config.
func Load(path string) (File, error) {
	return load(path)
}

func load(path string) (File, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, nil
	}
	if err != nil {
		return File{}, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

// Save writes f to path (0600), creating the parent directory.
func Save(path string, f File) error {
	mu.Lock()
	defer mu.Unlock()
	return saveLocked(path, f)
}

// Update loads path, applies mut, and writes the result back.
func Update(path string, mut func(*File)) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := load(path)
	if err != nil {
		return err
	}
	mut(&f)
	return saveLocked(path, f)
}

func saveLocked(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
