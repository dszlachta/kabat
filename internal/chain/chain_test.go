package chain_test

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

func startRPC(t *testing.T, version string, eth *ethAPI) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("web3", &web3API{version: version}))
	if eth == nil {
		eth = &ethAPI{chainID: big.NewInt(31337)}
	}
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestConnectURL_acceptsAnvil(t *testing.T) {
	srv := startRPC(t, "anvil/v1.0.0", nil)
	client, err := chain.ConnectURL(context.Background(), srv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
}

func TestConnectURL_acceptsAnvilCaseInsensitive(t *testing.T) {
	srv := startRPC(t, "Anvil/0.2.0", nil)
	client, err := chain.ConnectURL(context.Background(), srv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
}

func TestConnectURL_rejectsGeth(t *testing.T) {
	srv := startRPC(t, "Geth/v1.13.0", nil)
	_, err := chain.ConnectURL(context.Background(), srv.URL)
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
	require.EqualError(t, err, "cannot connect to local Anvil chain")
}

func TestConnectURL_rejectsHardhat(t *testing.T) {
	srv := startRPC(t, "hardhat", nil)
	_, err := chain.ConnectURL(context.Background(), srv.URL)
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func TestConnectURL_rejectsUnreachable(t *testing.T) {
	_, err := chain.ConnectURL(context.Background(), "http://127.0.0.1:1")
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func TestClient_BalanceAt(t *testing.T) {
	addr := wallets.Accounts()[0].Address
	want := new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e18))
	srv := startRPC(t, "anvil/v1.0.0", &ethAPI{
		chainID:  big.NewInt(31337),
		balances: map[common.Address]*big.Int{addr: want},
	})
	client, err := chain.ConnectURL(context.Background(), srv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	got, err := client.BalanceAt(context.Background(), addr)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestWebsocketURL(t *testing.T) {
	require.Equal(t, "ws://127.0.0.1:8545", chain.WebsocketURL("http://127.0.0.1:8545"))
	require.Equal(t, "wss://example:8545", chain.WebsocketURL("https://example:8545"))
	require.Equal(t, "ws://127.0.0.1:8545", chain.WebsocketURL("ws://127.0.0.1:8545"))
}

func TestClient_BalancesAt_batches(t *testing.T) {
	addrs := []common.Address{
		wallets.Accounts()[0].Address,
		wallets.Accounts()[1].Address,
		wallets.Accounts()[2].Address,
	}
	balances := map[common.Address]*big.Int{
		addrs[0]: big.NewInt(1),
		addrs[1]: big.NewInt(2),
		addrs[2]: big.NewInt(3),
	}
	rpcSrv := rpc.NewServer()
	t.Cleanup(rpcSrv.Stop)
	require.NoError(t, rpcSrv.RegisterName("web3", &web3API{version: "anvil/v1.0.0"}))
	require.NoError(t, rpcSrv.RegisterName("eth", &ethAPI{chainID: big.NewInt(31337), balances: balances}))

	var requests atomic.Int32
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		rpcSrv.ServeHTTP(w, r)
	}))
	t.Cleanup(httpSrv.Close)

	client, err := chain.ConnectURL(context.Background(), httpSrv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	require.Equal(t, int32(1), requests.Load(), "connect should be one HTTP request")

	got, err := client.BalancesAt(context.Background(), addrs)
	require.NoError(t, err)
	require.Equal(t, int32(2), requests.Load(), "balances should be one batched HTTP request")
	require.Equal(t, []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}, got)
}
