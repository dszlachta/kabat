package walletservice_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/config"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestConfig_persistsWalletNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	srv := startAnvil(t, &ethAPI{})
	svc := walletservice.NewWithConfig(srv.URL, path)
	addr := wallets.Addresses()[3]
	require.NoError(t, svc.SetAccountName(addr, "Alice"))

	svc2 := walletservice.NewWithConfig(srv.URL, path)
	got, err := svc2.Accounts()
	require.NoError(t, err)
	require.Equal(t, "Alice", got[3].Name)
	require.Empty(t, got[0].Name)
}

func TestConfig_nameSaveKeepsWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, config.Save(path, config.File{
		Window: &config.Window{Width: 1330, Height: 810},
	}))
	srv := startAnvil(t, &ethAPI{})
	svc := walletservice.NewWithConfig(srv.URL, path)
	require.NoError(t, svc.SetAccountName(wallets.Addresses()[0], "Alice"))
	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, 1330, got.Window.Width)
	require.Equal(t, 810, got.Window.Height)
	require.Equal(t, "Alice", got.Wallets[wallets.Addresses()[0]].Name)
}

func TestConfig_clearNameOmitsWallet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	srv := startAnvil(t, &ethAPI{})
	svc := walletservice.NewWithConfig(srv.URL, path)
	addr := wallets.Addresses()[1]
	require.NoError(t, svc.SetAccountName(addr, "Bob"))
	require.NoError(t, svc.SetAccountName(addr, "  "))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f config.File
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Empty(t, f.Wallets)
}

func TestConfig_reloadsABIForSameAddress(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	path := filepath.Join(t.TempDir(), "config.json")
	addr := common.HexToAddress("0x7777777777777777777777777777777777777777")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithConfig(srv.URL, path)
	abiPath := filepath.Join("..", "contracts", "testdata", "abi.json")
	svc.SetPickJSONFile(func() (string, error) { return abiPath, nil })
	_, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f config.File
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Contains(t, f.Contracts[addr.Hex()].ABI, "abi.json")
	require.True(t, filepath.IsAbs(f.Contracts[addr.Hex()].ABI))

	contracts.Reset()
	svc2 := walletservice.NewWithConfig(srv.URL, path)
	got := svc2.ContractInfo(addr.Hex())
	require.Contains(t, got.ABI, `"name"`)
}

func TestConfig_missingABIFileIsIgnored(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	path := filepath.Join(t.TempDir(), "config.json")
	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, config.Save(path, config.File{
		Contracts: map[string]config.Contract{
			addr.Hex(): {ABI: filepath.Join(t.TempDir(), "gone.json")},
		},
	}))
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithConfig(srv.URL, path)
	got := svc.ContractInfo(addr.Hex())
	require.Empty(t, got.ABI)
}

func TestConfig_cancelDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	addr := common.HexToAddress("0x6666666666666666666666666666666666666666")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0x60, 0x00}},
	})
	svc := walletservice.NewWithConfig(srv.URL, path)
	svc.SetPickJSONFile(func() (string, error) { return "", nil })
	_, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	_, err = os.Stat(path)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestConfig_persistsAnvilURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	svc := walletservice.NewWithConfig(chain.DefaultURL, path)
	require.NoError(t, svc.SetAnvilURL("http://127.0.0.1:8546"))

	got := svc.Settings()
	require.Equal(t, "http://127.0.0.1:8546", got.AnvilURL)
	require.Equal(t, chain.DefaultURL, got.DefaultAnvilURL)

	svc2 := walletservice.NewWithConfig("http://ignored.example:1", path)
	require.Equal(t, "http://127.0.0.1:8546", svc2.Settings().AnvilURL)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f config.File
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Equal(t, "http://127.0.0.1:8546", f.AnvilURL)
}

func TestConfig_defaultAnvilURLOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	svc := walletservice.NewWithConfig(chain.DefaultURL, path)
	require.NoError(t, svc.SetAnvilURL("http://127.0.0.1:8546"))
	require.NoError(t, svc.SetAnvilURL(""))
	require.Equal(t, chain.DefaultURL, svc.Settings().AnvilURL)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f config.File
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Empty(t, f.AnvilURL)
}

func TestSetMCP_invokesCallback(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	var got []bool
	svc.SetOnMCP(func(on bool) error {
		got = append(got, on)
		return nil
	})
	require.NoError(t, svc.SetMCP(true))
	require.NoError(t, svc.SetMCP(false))
	require.Equal(t, []bool{true, false}, got)
}

func TestSetMCP_callbackErrorReverts(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	svc.SetOnMCP(func(bool) error { return errors.New("listen failed") })
	require.Error(t, svc.SetMCP(true))
	require.False(t, svc.Settings().MCP)
}

func TestSettings_includesMCPURL(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.Equal(t, config.MCPURL(), svc.Settings().MCPURL)
}

func TestConfig_persistsMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	svc := walletservice.NewWithConfig(chain.DefaultURL, path)
	require.False(t, svc.Settings().MCP)
	require.NoError(t, svc.SetMCP(true))
	require.True(t, svc.Settings().MCP)

	svc2 := walletservice.NewWithConfig(chain.DefaultURL, path)
	require.True(t, svc2.Settings().MCP)

	require.NoError(t, svc.SetMCP(false))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var f config.File
	require.NoError(t, json.Unmarshal(raw, &f))
	require.False(t, f.MCP)
}

func TestConfig_nameSaveKeepsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	svc := walletservice.NewWithConfig(chain.DefaultURL, path)
	require.NoError(t, svc.SetAnvilURL("http://127.0.0.1:8546"))
	require.NoError(t, svc.SetMCP(true))
	require.NoError(t, svc.SetAccountName(wallets.Addresses()[0], "Alice"))

	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8546", got.AnvilURL)
	require.True(t, got.MCP)
	require.Equal(t, "Alice", got.Wallets[wallets.Addresses()[0]].Name)
}

func TestSetAnvilURL_invalid(t *testing.T) {
	svc := walletservice.NewWithURL(chain.DefaultURL)
	require.ErrorIs(t, svc.SetAnvilURL("not a url"), walletservice.ErrInvalidAnvilURL)
	require.Equal(t, chain.DefaultURL, svc.Settings().AnvilURL)
}

func TestNewWithURL_doesNotWriteConfig(t *testing.T) {
	path := config.DefaultPath()
	before, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		require.NoError(t, err)
	}
	srv := startAnvil(t, &ethAPI{})
	svc := walletservice.NewWithURL(srv.URL)
	require.NoError(t, svc.SetAccountName(wallets.Addresses()[0], "ShouldNotPersist"))
	if missing {
		_, err := os.Stat(path)
		require.True(t, errors.Is(err, os.ErrNotExist))
		return
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
