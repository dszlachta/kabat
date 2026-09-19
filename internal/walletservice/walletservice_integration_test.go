//go:build integration

package walletservice_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestAccounts_liveAnvil(t *testing.T) {
	got, err := walletservice.New().Accounts()
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	require.Len(t, got, 10)

	want := wallets.Addresses()
	for i, acc := range got {
		require.Equal(t, want[i], acc.Address)
		require.NotEmpty(t, acc.Balance)
		require.Equal(t, i == 0, acc.Selected)
	}
}

func TestRequest_liveSendTransactionConfirm(t *testing.T) {
	svc := walletservice.New()
	pending := make(chan struct{})
	svc.SetEmitter(func(string, any) { close(pending) })

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"from":  wallets.Addresses()[0],
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		hash, ok := got.v.(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(hash, "0x"))
		require.Len(t, hash, 66)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestStartWatch_liveNewHeadEmitsAccounts(t *testing.T) {
	svc := walletservice.New()
	got := make(chan []walletservice.Account, 4)
	svc.SetEmitter(func(event string, data any) {
		if event != "accounts" {
			return
		}
		accs, ok := data.([]walletservice.Account)
		if ok {
			got <- accs
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc.StartWatch(ctx)

	time.Sleep(500 * time.Millisecond)
	client, err := chain.Connect(context.Background())
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	t.Cleanup(client.Close)
	require.NoError(t, client.RPC().CallContext(context.Background(), nil, "evm_mine"))

	select {
	case accs := <-got:
		require.Len(t, accs, 10)
		require.NotEmpty(t, accs[0].Balance)
	case <-ctx.Done():
		t.Fatal("timeout waiting for accounts emit after new head")
	}
}

func liveService(t *testing.T) *walletservice.WalletService {
	t.Helper()
	client, err := chain.Connect(context.Background())
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	client.Close()
	return walletservice.NewWithURL(chain.DefaultURL)
}

func startWatch(t *testing.T, svc *walletservice.WalletService) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.StartWatch(ctx)
	time.Sleep(500 * time.Millisecond)
}

func pendingEmitter(t *testing.T, svc *walletservice.WalletService) <-chan struct{} {
	t.Helper()
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	return pending
}

func waitConfirm(t *testing.T, svc *walletservice.WalletService, pending <-chan struct{}) {
	t.Helper()
	select {
	case <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())
}

func confirmRequest(t *testing.T, svc *walletservice.WalletService, method string, params any) any {
	t.Helper()
	pending := pendingEmitter(t, svc)
	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request(method, params)
		result <- struct {
			v   any
			err error
		}{v, err}
	}()
	waitConfirm(t, svc, pending)
	select {
	case got := <-result:
		require.NoError(t, got.err)
		return got.v
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request result")
		return nil
	}
}

func confirmHash(t *testing.T, svc *walletservice.WalletService, fn func() (string, error)) string {
	t.Helper()
	pending := pendingEmitter(t, svc)
	result := make(chan struct {
		v   string
		err error
	}, 1)
	go func() {
		v, err := fn()
		result <- struct {
			v   string
			err error
		}{v, err}
	}()
	waitConfirm(t, svc, pending)
	select {
	case got := <-result:
		require.NoError(t, got.err)
		return got.v
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for send result")
		return ""
	}
}

func confirmCall(t *testing.T, svc *walletservice.WalletService, fn func() (walletservice.ContractCallResult, error)) walletservice.ContractCallResult {
	t.Helper()
	pending := pendingEmitter(t, svc)
	result := make(chan struct {
		v   walletservice.ContractCallResult
		err error
	}, 1)
	go func() {
		v, err := fn()
		result <- struct {
			v   walletservice.ContractCallResult
			err error
		}{v, err}
	}()
	waitConfirm(t, svc, pending)
	select {
	case got := <-result:
		require.NoError(t, got.err)
		return got.v
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for call result")
		return walletservice.ContractCallResult{}
	}
}

func waitReceipt(t *testing.T, client *chain.Client, hash string) *types.Receipt {
	t.Helper()
	ctx := context.Background()
	txHash := common.HexToHash(hash)
	var last error
	for range 20 {
		_ = client.RPC().CallContext(ctx, nil, "evm_mine")
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil && receipt != nil {
			return receipt
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	require.NoError(t, last, "receipt for %s", hash)
	return nil
}

func TestRequest_liveSignTypedDataConfirm(t *testing.T) {
	svc := liveService(t)
	client, err := chain.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(context.Background())
	require.NoError(t, err)
	from := wallets.Addresses()[0]

	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_signTypedData_v4", []any{from, map[string]any{
			"types": map[string]any{
				"EIP712Domain": []any{
					map[string]any{"name": "name", "type": "string"},
					map[string]any{"name": "version", "type": "string"},
					map[string]any{"name": "chainId", "type": "uint256"},
					map[string]any{"name": "verifyingContract", "type": "address"},
				},
				"Mail": []any{
					map[string]any{"name": "contents", "type": "string"},
				},
			},
			"primaryType": "Mail",
			"domain": map[string]any{
				"name":              "Ether Mail",
				"version":           "1",
				"chainId":           chainID.Int64(),
				"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC",
			},
			"message": map[string]any{"contents": "Hello, Bob!"},
		}})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		sig, ok := got.v.(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(sig, "0x"))
		require.Len(t, sig, 132)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestSendETHAuto_live(t *testing.T) {
	svc := liveService(t)
	hash, err := svc.SendETHAuto(wallets.Addresses()[0], wallets.Addresses()[1], "0x1")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "0x"))
	require.Len(t, hash, 66)

	client, err := chain.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	receipt := waitReceipt(t, client, hash)
	require.Equal(t, uint64(1), receipt.Status)
}

func TestCallContractAuto_liveView(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	svc := liveService(t)
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"from": wallets.Addresses()[0],
			"data": "0x600a80600b6000396000f3602a60005260206000f3",
		}})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()
	select {
	case <-pending:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())
	var hash string
	select {
	case got := <-result:
		require.NoError(t, got.err)
		var ok bool
		hash, ok = got.v.(string)
		require.True(t, ok)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for deploy result")
	}

	client, err := chain.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	receipt := waitReceipt(t, client, hash)
	require.NotEqual(t, common.Address{}, receipt.ContractAddress)

	abiPath := filepath.Join("..", "contracts", "testdata", "abi_members.json")
	_, err = svc.LoadContractABIFromPath(receipt.ContractAddress.Hex(), abiPath)
	require.NoError(t, err)

	got, err := svc.CallContractAuto(receipt.ContractAddress.Hex(), "0x70a08231", []string{
		wallets.Addresses()[0],
	}, "", "")
	require.NoError(t, err)
	require.Equal(t, "view", got.Kind)
	require.Empty(t, got.Hash)
	require.Equal(t, []walletservice.TxArg{{Type: "uint256", Text: "42"}}, got.Outputs)
}

func TestStartWatch_liveNewHeadEmitsTransactions(t *testing.T) {
	svc := liveService(t)
	got := make(chan []walletservice.StoredTx, 8)
	svc.SetEmitter(func(event string, data any) {
		if event != "transactions" {
			return
		}
		txs, ok := data.([]walletservice.StoredTx)
		if ok {
			got <- txs
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	svc.StartWatch(ctx)

	time.Sleep(500 * time.Millisecond)
	hash, err := svc.SendETHAuto(wallets.Addresses()[0], wallets.Addresses()[1], "0x1")
	require.NoError(t, err)

	client, err := chain.Connect(context.Background())
	require.NoError(t, err)
	t.Cleanup(client.Close)
	require.NoError(t, client.RPC().CallContext(context.Background(), nil, "evm_mine"))

	for {
		select {
		case txs := <-got:
			for _, tx := range txs {
				if strings.EqualFold(tx.Hash, hash) {
					require.Equal(t, "transfer", tx.Kind)
					return
				}
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for transactions emit after send")
		}
	}
}
