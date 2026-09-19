package walletservice_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type web3API struct {
	version string
}

func (a *web3API) ClientVersion() string {
	return a.version
}

type ethAPI struct {
	chainID  *big.Int
	balances map[common.Address]*big.Int
	nonce    hexutil.Uint64
	gas      hexutil.Uint64
	gasPrice *big.Int
}

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *ethAPI) GetBalance(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	if a.balances == nil {
		return (*hexutil.Big)(big.NewInt(0)), nil
	}
	if bal, ok := a.balances[address]; ok {
		return (*hexutil.Big)(new(big.Int).Set(bal)), nil
	}
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (a *ethAPI) GetTransactionCount(common.Address, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return a.nonce
}

func (a *ethAPI) EstimateGas(map[string]any) hexutil.Uint64 {
	if a.gas == 0 {
		return 21000
	}
	return a.gas
}

func (a *ethAPI) GasPrice() *hexutil.Big {
	if a.gasPrice == nil {
		return (*hexutil.Big)(big.NewInt(1e9))
	}
	return (*hexutil.Big)(a.gasPrice)
}

func (a *ethAPI) SendRawTransaction(data hexutil.Bytes) common.Hash {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}
	}
	return tx.Hash()
}

func startAnvil(t *testing.T, eth *ethAPI) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("web3", &web3API{version: "anvil/v1.0.0"}))
	if eth == nil {
		eth = &ethAPI{chainID: big.NewInt(31337)}
	}
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestSelect_defaultsToFirstAccount(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.Equal(t, wallets.Addresses()[0], svc.Selected())
}

func TestSelect_updatesSelected(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	want := wallets.Addresses()[2]
	require.NoError(t, svc.Select(want))
	require.Equal(t, want, svc.Selected())
}

func TestSelect_rejectsUnknown(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	err := svc.Select("0x0000000000000000000000000000000000000001")
	require.Error(t, err)
}

func TestSelect_notifiesOnSelected(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	var got string
	svc.SetOnSelected(func() { got = svc.Selected() })
	want := wallets.Addresses()[2]
	require.NoError(t, svc.Select(want))
	require.Equal(t, want, got)
}

func TestSelect_broadcastsAccountsChanged(t *testing.T) {
	httpSrv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(httpSrv.URL)
	ipcSrv := ipc.NewServer(svc)
	svc.SetOnSelected(func() { ipcSrv.AccountsChanged() })

	a, b := net.Pipe()
	t.Cleanup(func() {
		// Test teardown; the peer may already be closed.
		_ = a.Close()
		_ = b.Close()
	})
	go ipcSrv.ServeConn(a)

	require.NoError(t, b.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := ipc.ReadEnvelope(b)
	require.NoError(t, err)

	want := wallets.Addresses()[2]
	errc := make(chan error, 1)
	go func() { errc <- svc.Select(want) }()

	env, err := ipc.ReadEnvelope(b)
	require.NoError(t, err)
	require.NoError(t, <-errc)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	require.Equal(t, "accountsChanged", payload.Event)
	list, ok := payload.Params.([]any)
	require.True(t, ok)
	require.Equal(t, want, list[0])
}

func TestWalletService_ipcHandler(t *testing.T) {
	var _ ipc.Handler = walletservice.NewWithURL("http://127.0.0.1:1")
}

func TestAccounts_returnsAddressesAndBalances(t *testing.T) {
	ether := new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	balances := make(map[common.Address]*big.Int)
	for i, acc := range wallets.Accounts() {
		balances[acc.Address] = new(big.Int).Mul(ether, big.NewInt(int64(i+1)))
	}
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337), balances: balances})
	svc := walletservice.NewWithURL(srv.URL)

	got, err := svc.Accounts()
	require.NoError(t, err)
	require.Len(t, got, 10)
	for i, acc := range wallets.Accounts() {
		require.Equal(t, acc.Address.Hex(), got[i].Address)
		require.Equal(t, new(big.Int).Mul(big.NewInt(int64(i+1)), big.NewInt(10000)).String(), got[i].Balance)
		require.Equal(t, i == 0, got[i].Selected)
	}

	require.NoError(t, svc.Select(wallets.Addresses()[1]))
	got, err = svc.Accounts()
	require.NoError(t, err)
	require.False(t, got[0].Selected)
	require.True(t, got[1].Selected)
}

func TestAccounts_mapsConnectionErrors(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.Accounts()
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func TestFormatEther(t *testing.T) {
	require.Equal(t, "0", walletservice.FormatEther(big.NewInt(0)))
	require.Equal(t, "1", walletservice.FormatEther(big.NewInt(params.Ether)))
	require.Equal(t, "10000", walletservice.FormatEther(new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))))
	require.Equal(t, "1.5", walletservice.FormatEther(new(big.Int).Mul(big.NewInt(15), big.NewInt(params.Ether/10))))
}

func TestConfirmPending_withoutPending(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.ErrorIs(t, svc.ConfirmPending(), walletservice.ErrNoPendingTransaction)
	require.ErrorIs(t, svc.CancelPending(), walletservice.ErrNoPendingTransaction)
}

func TestRequest_mapsConnectionErrors(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.Request("eth_chainId", nil)
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func TestRequest_sendTransactionConfirm(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		require.Equal(t, "pendingTransaction", event)
		pending <- data
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case raw := <-pending:
		require.IsType(t, map[string]any{}, raw)
	case <-time.After(2 * time.Second):
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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestRequest_sendTransactionCancel(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)

	pending := make(chan struct{})
	svc.SetEmitter(func(string, any) { close(pending) })

	result := make(chan error, 1)
	go func() {
		_, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}})
		result <- err
	}()

	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.CancelPending())

	select {
	case err := <-result:
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4001, rpcErr.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}
