package walletservice

import (
	"context"
	"math/big"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type watchWeb3API struct {
	version string
}

func (a *watchWeb3API) ClientVersion() string {
	return a.version
}

type watchEthAPI struct {
	chainID  *big.Int
	balances map[common.Address]*big.Int
}

func (a *watchEthAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *watchEthAPI) GetBalance(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	if a.balances == nil {
		return (*hexutil.Big)(big.NewInt(0)), nil
	}
	if bal, ok := a.balances[address]; ok {
		return (*hexutil.Big)(new(big.Int).Set(bal)), nil
	}
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func TestWatchHeads_emitsAccounts(t *testing.T) {
	ether := new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	balances := make(map[common.Address]*big.Int)
	for _, acc := range wallets.Accounts() {
		balances[acc.Address] = ether
	}
	rpcSrv := rpc.NewServer()
	t.Cleanup(rpcSrv.Stop)
	require.NoError(t, rpcSrv.RegisterName("web3", &watchWeb3API{version: "anvil/v1.0.0"}))
	require.NoError(t, rpcSrv.RegisterName("eth", &watchEthAPI{chainID: big.NewInt(31337), balances: balances}))
	httpSrv := httptest.NewServer(rpcSrv)
	t.Cleanup(httpSrv.Close)

	svc := NewWithURL(httpSrv.URL)
	got := make(chan []Account, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "accounts" {
			got <- data.([]Account)
		}
	})

	heads := make(chan *types.Header, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.watchHeads(ctx, heads, nil) }()

	heads <- &types.Header{Number: big.NewInt(1)}
	select {
	case accs := <-got:
		require.Len(t, accs, 10)
		require.Equal(t, wallets.Addresses()[0], accs[0].Address)
		require.Equal(t, "10000", accs[0].Balance)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for accounts emit")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for watchHeads to exit")
	}
}
