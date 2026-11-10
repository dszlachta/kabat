package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/config"
	"github.com/stretchr/testify/require"
)

func TestLoad_missingIsEmpty(t *testing.T) {
	got, err := config.Load(filepath.Join(t.TempDir(), "missing.json"))
	require.NoError(t, err)
	require.Empty(t, got.Wallets)
	require.Empty(t, got.Contracts)
}

func TestSaveLoad_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := config.File{
		Window:   &config.Window{Width: 1200, Height: 800},
		AnvilURL: "http://127.0.0.1:8546",
		MCP:      true,
		Wallets: map[string]config.Wallet{
			"0x1111111111111111111111111111111111111111": {Name: "Alice"},
		},
		Contracts: map[string]config.Contract{
			"0x2222222222222222222222222222222222222222": {ABI: "/tmp/abi.json"},
		},
	}
	require.NoError(t, config.Save(path, want))
	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestUpdate_preservesOtherFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, config.Save(path, config.File{
		Wallets: map[string]config.Wallet{
			"0x1111111111111111111111111111111111111111": {Name: "Alice"},
		},
	}))
	require.NoError(t, config.Update(path, func(f *config.File) {
		f.Window = &config.Window{Width: 1440, Height: 900}
	}))
	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "Alice", got.Wallets["0x1111111111111111111111111111111111111111"].Name)
	require.Equal(t, 1440, got.Window.Width)
	require.Equal(t, 900, got.Window.Height)
}

func TestDefaultPath_inKabatDir(t *testing.T) {
	require.Equal(t, "config.json", filepath.Base(config.DefaultPath()))
	require.Equal(t, "kabat", filepath.Base(filepath.Dir(config.DefaultPath())))
}

func TestSave_createsParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	require.NoError(t, config.Save(path, config.File{}))
	_, err := os.Stat(path)
	require.NoError(t, err)
}
