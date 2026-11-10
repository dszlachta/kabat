//go:build integration

package walletservice_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

// WETH9 from gnosis/canonical-weth@0dd1ea3e295eef916d0c6223ec63141137d22d67 (build/contracts/WETH9.json).
func weth9Artifact(t *testing.T) (bytecode string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "contracts", "testdata", "weth9.json"))
	require.NoError(t, err)
	var art struct {
		Bytecode string `json:"bytecode"`
	}
	require.NoError(t, json.Unmarshal(raw, &art))
	require.True(t, strings.HasPrefix(art.Bytecode, "0x"))
	return art.Bytecode
}

func waitContractsContains(t *testing.T, svc *walletservice.WalletService, client *chain.Client, addr string) walletservice.StoredContract {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		_ = client.RPC().CallContext(context.Background(), nil, "evm_mine")
		for _, c := range svc.Contracts() {
			if strings.EqualFold(c.Address, addr) {
				return c
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timeout waiting for contract ingest")
	return walletservice.StoredContract{}
}

func infoHasFunc(info walletservice.ContractInfo, name string) bool {
	for _, fn := range info.Functions {
		if fn.Name == name {
			return true
		}
	}
	return false
}

func infoHasEvent(info walletservice.ContractInfo, name string) bool {
	for _, ev := range info.Events {
		if ev.Name == name {
			return true
		}
	}
	return false
}

func TestWETH_live(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)

	svc := liveService(t)
	startWatch(t, svc)
	client, err := chain.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(client.Close)

	from := wallets.Addresses()[0]
	to := wallets.Addresses()[1]
	abiPath := filepath.Join("..", "contracts", "testdata", "weth9.json")

	hash, ok := confirmRequest(t, svc, "eth_sendTransaction", []any{map[string]any{
		"from": from,
		"data": weth9Artifact(t),
	}}).(string)
	require.True(t, ok)
	receipt := waitReceipt(t, client, hash)
	require.NotEqual(t, common.Address{}, receipt.ContractAddress)
	addr := receipt.ContractAddress.Hex()

	t.Run("ingest", func(t *testing.T) {
		got := waitContractsContains(t, svc, client, addr)
		require.Equal(t, hash, got.TxHash)
	})

	t.Run("details", func(t *testing.T) {
		got, err := svc.ContractDetails(addr)
		require.NoError(t, err)
		require.Equal(t, from, got.Deployer)
		require.Equal(t, hash, got.Hash)
		require.Greater(t, got.CodeSize, 0)
		require.True(t, strings.HasPrefix(got.Bytecode, "0x"))
		require.False(t, got.ABILoaded)
	})

	t.Run("abi", func(t *testing.T) {
		info, err := svc.LoadContractABIFromPath(addr, abiPath)
		require.NoError(t, err)
		require.True(t, infoHasFunc(info, "deposit"))
		require.True(t, infoHasFunc(info, "withdraw"))
		require.True(t, infoHasFunc(info, "transfer"))
		require.True(t, infoHasFunc(info, "balanceOf"))
		require.True(t, infoHasEvent(info, "Transfer"))
		require.Equal(t, "deposit", svc.ABISelectors()["0xd0e30db0"])

		details, err := svc.ContractDetails(addr)
		require.NoError(t, err)
		require.True(t, details.ABILoaded)
		require.Greater(t, details.Functions, 0)
		require.GreaterOrEqual(t, details.Events, 3)
	})

	t.Run("tokens", func(t *testing.T) {
		got, err := svc.Tokens()
		require.NoError(t, err)
		var tok *walletservice.ERC20Token
		for i := range got {
			if strings.EqualFold(got[i].Address, addr) {
				tok = &got[i]
				break
			}
		}
		require.NotNil(t, tok)
		require.Equal(t, "WETH", tok.Symbol)
		require.Equal(t, uint8(18), tok.Decimals)
		require.Equal(t, "Wrapped Ether", tok.Name)
	})

	t.Run("sendERC20NoBalance", func(t *testing.T) {
		_, err := svc.SendERC20Auto(from, addr, to, "1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "insufficient WETH balance")
	})

	t.Run("deposit", func(t *testing.T) {
		svc.SetEmitter(func(event string, data any) {
			if event == "pendingTransaction" {
				t.Errorf("unexpected pendingTransaction")
			}
		})
		got, err := svc.CallContractAuto(addr, selectorNamed(t, svc, "deposit"), nil, from, "0x1")
		require.NoError(t, err)
		require.Equal(t, "send", got.Kind)
		require.True(t, strings.HasPrefix(got.Hash, "0x"))
		waitReceipt(t, client, got.Hash)

		bal, err := svc.CallContractAuto(addr, selectorNamed(t, svc, "balanceOf"), []string{from}, from, "")
		require.NoError(t, err)
		require.Equal(t, "view", bal.Kind)
		require.Equal(t, []walletservice.TxArg{{
			Type: "uint256",
			Text: "1",
		}}, bal.Outputs)
	})

	t.Run("erc20Details", func(t *testing.T) {
		got, err := svc.ContractDetails(addr)
		require.NoError(t, err)
		require.NotNil(t, got.ERC20)
		require.Equal(t, "Wrapped Ether", got.ERC20.Name)
		require.Equal(t, "WETH", got.ERC20.Symbol)
		require.Equal(t, uint8(18), got.ERC20.Decimals)
		require.Equal(t, "0.000000000000000001", got.ERC20.TotalSupply)
	})

	var transferHash string
	t.Run("transfer", func(t *testing.T) {
		got := confirmCall(t, svc, func() (walletservice.ContractCallResult, error) {
			return svc.CallContract(addr, selectorNamed(t, svc, "transfer"), []string{to, "1"}, from, "")
		})
		require.Equal(t, "send", got.Kind)
		require.True(t, strings.HasPrefix(got.Hash, "0x"))
		transferHash = got.Hash
		receipt := waitReceipt(t, client, transferHash)
		require.Equal(t, uint64(1), receipt.Status)
	})

	t.Run("txDecode", func(t *testing.T) {
		require.NotEmpty(t, transferHash)
		require.NotEmpty(t, svc.Transactions())
		details, err := svc.TxDetails(transferHash)
		require.NoError(t, err)
		require.True(t, details.Ok)
		require.Equal(t, "call", details.Kind)
		require.Equal(t, "transfer", details.Method)
		require.Len(t, details.Args, 2)
		require.Equal(t, "address", details.Args[0].Type)
		require.Equal(t, to, details.Args[0].Text)
		require.Equal(t, "uint256", details.Args[1].Type)
		require.Equal(t, "1", details.Args[1].Text)
		var transferLog *walletservice.TxLog
		for i := range details.Logs {
			if details.Logs[i].Name == "Transfer" {
				transferLog = &details.Logs[i]
				break
			}
		}
		require.NotNil(t, transferLog)
		require.Len(t, transferLog.Args, 3)
		require.Equal(t, from, transferLog.Args[0].Text)
		require.Equal(t, to, transferLog.Args[1].Text)
		require.Equal(t, "1", transferLog.Args[2].Text)

		var named bool
		for _, tx := range svc.Transactions() {
			if strings.EqualFold(tx.Hash, transferHash) {
				require.Equal(t, "transfer", tx.Method)
				named = true
			}
		}
		require.True(t, named)
	})

	recipient := wallets.Addresses()[2]
	t.Run("sendERC20", func(t *testing.T) {
		svc.SetEmitter(func(event string, data any) {
			if event == "pendingTransaction" {
				t.Errorf("unexpected pendingTransaction")
			}
		})
		dep, err := svc.CallContractAuto(addr, selectorNamed(t, svc, "deposit"), nil, from, "0x1bc16d674ec80000")
		require.NoError(t, err)
		waitReceipt(t, client, dep.Hash)

		hash := confirmHash(t, svc, func() (string, error) {
			return svc.SendERC20(addr, recipient, "1")
		})
		require.True(t, strings.HasPrefix(hash, "0x"))
		waitReceipt(t, client, hash)

		bal, err := svc.CallContractAuto(addr, selectorNamed(t, svc, "balanceOf"), []string{recipient}, from, "")
		require.NoError(t, err)
		require.Equal(t, "view", bal.Kind)
		require.Equal(t, []walletservice.TxArg{{
			Type: "uint256",
			Text: "1000000000000000000",
		}}, bal.Outputs)
	})

	t.Run("sendERC20Auto", func(t *testing.T) {
		svc.SetEmitter(func(event string, data any) {
			if event == "pendingTransaction" {
				t.Errorf("unexpected pendingTransaction")
			}
		})
		hash, err := svc.SendERC20Auto(from, addr, recipient, "1")
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(hash, "0x"))
		waitReceipt(t, client, hash)

		bal, err := svc.CallContractAuto(addr, selectorNamed(t, svc, "balanceOf"), []string{recipient}, from, "")
		require.NoError(t, err)
		require.Equal(t, []walletservice.TxArg{{
			Type: "uint256",
			Text: "2000000000000000000",
		}}, bal.Outputs)
	})

	t.Run("revert", func(t *testing.T) {
		c, ok := contracts.Get(common.HexToAddress(addr))
		require.True(t, ok)
		packed, _, err := c.Pack(selectorNamed(t, svc, "transfer"), []string{to, "1"})
		require.NoError(t, err)
		hash, ok := confirmRequest(t, svc, "eth_sendTransaction", []any{map[string]any{
			"from": from,
			"to":   addr,
			"data": hexutil.Encode(packed),
			"gas":  "0x186a0",
		}}).(string)
		require.True(t, ok)
		receipt := waitReceipt(t, client, hash)
		require.Equal(t, uint64(0), receipt.Status)
		require.NotEmpty(t, svc.Transactions())
		details, err := svc.TxDetails(hash)
		require.NoError(t, err)
		require.False(t, details.Ok)
		require.NotEmpty(t, details.Revert)
	})
}
