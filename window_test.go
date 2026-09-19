package main

import (
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDefaultWindowSize(t *testing.T) {
	tests := []struct {
		name         string
		workW, workH int
		wantW, wantH int
	}{
		{name: "13 inch", workW: 1440, workH: 845, wantW: 800, wantH: 600},
		{name: "1080p", workW: 1920, workH: 1040, wantW: 853, wantH: 600},
		{name: "1440p", workW: 2560, workH: 1400, wantW: 1137, wantH: 700},
		{name: "4K", workW: 3840, workH: 2160, wantW: 1706, wantH: 1080},
		{name: "tiny work area", workW: 700, workH: 500, wantW: 700, wantH: 500},
		{name: "unknown screen", workW: 0, workH: 0, wantW: 800, wantH: 600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := defaultWindowSize(tt.workW, tt.workH)
			require.Equal(t, tt.wantW, gotW)
			require.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestMinWindowSize(t *testing.T) {
	tests := []struct {
		name         string
		workW, workH int
		wantW, wantH int
	}{
		{name: "large screen", workW: 1920, workH: 1080, wantW: 800, wantH: 600},
		{name: "tiny work area", workW: 700, workH: 500, wantW: 700, wantH: 500},
		{name: "unknown screen", workW: 0, workH: 0, wantW: 800, wantH: 600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := minWindowSize(tt.workW, tt.workH)
			require.Equal(t, tt.wantW, gotW)
			require.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestRestoreWindowSize(t *testing.T) {
	tests := []struct {
		name           string
		workW, workH   int
		savedW, savedH int
		wantW, wantH   int
	}{
		{name: "no saved uses default", workW: 2560, workH: 1400, wantW: 1137, wantH: 700},
		{name: "saved size", workW: 1920, workH: 1040, savedW: 1200, savedH: 800, wantW: 1200, wantH: 800},
		{name: "saved below min", workW: 1920, workH: 1080, savedW: 400, savedH: 300, wantW: 800, wantH: 600},
		{name: "saved bigger than work area", workW: 1920, workH: 1040, savedW: 3000, savedH: 2000, wantW: 1920, wantH: 1040},
		{name: "zero saved uses default", workW: 1920, workH: 1040, savedW: 0, savedH: 0, wantW: 853, wantH: 600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotW, gotH := restoreWindowSize(tt.workW, tt.workH, tt.savedW, tt.savedH)
			require.Equal(t, tt.wantW, gotW)
			require.Equal(t, tt.wantH, gotH)
		})
	}
}

func TestPersistWindowSize_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, persistWindowSize(path, 1280, 720))
	gotW, gotH := loadSavedWindowSize(path)
	require.Equal(t, 1280, gotW)
	require.Equal(t, 720, gotH)
}

func TestPersistWindowSize_keepsWallets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, config.Save(path, config.File{
		AnvilURL: "http://127.0.0.1:8546",
		MCP:      true,
		Wallets: map[string]config.Wallet{
			"0x1111111111111111111111111111111111111111": {Name: "Alice"},
		},
	}))
	require.NoError(t, persistWindowSize(path, 1100, 700))
	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, 1100, got.Window.Width)
	require.Equal(t, 700, got.Window.Height)
	require.Equal(t, "Alice", got.Wallets["0x1111111111111111111111111111111111111111"].Name)
	require.Equal(t, "http://127.0.0.1:8546", got.AnvilURL)
	require.True(t, got.MCP)
}

func TestPersistWindowSize_ignoresZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, persistWindowSize(path, 0, 0))
	gotW, gotH := loadSavedWindowSize(path)
	require.Equal(t, 0, gotW)
	require.Equal(t, 0, gotH)
}
