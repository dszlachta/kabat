//go:build integration

package provider_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func connectLive(t *testing.T) *chain.Client {
	t.Helper()
	client, err := chain.Connect(context.Background())
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	t.Cleanup(client.Close)
	return client
}

func TestRequest_liveEthAccounts(t *testing.T) {
	p := provider.New(connectLive(t), provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_accounts"})
	require.NoError(t, err)
	require.Equal(t, wallets.Addresses(), got)
}

func TestRequest_liveEthRequestAccounts(t *testing.T) {
	p := provider.New(connectLive(t), provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_requestAccounts"})
	require.NoError(t, err)
	require.Equal(t, wallets.Addresses(), got)
}

func TestRequest_liveEthChainId(t *testing.T) {
	client := connectLive(t)
	p := provider.New(client, provider.Options{})

	id, err := client.ChainID(context.Background())
	require.NoError(t, err)

	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_chainId"})
	require.NoError(t, err)
	require.Equal(t, hexutil.EncodeBig(id), got)
}

func TestRequest_liveEthGetBalance(t *testing.T) {
	client := connectLive(t)
	p := provider.New(client, provider.Options{})
	addr := wallets.Accounts()[0].Address

	wei, err := client.BalanceAt(context.Background(), addr)
	require.NoError(t, err)

	got, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_getBalance",
		Params: []any{addr.Hex(), "latest"},
	})
	require.NoError(t, err)
	require.Equal(t, hexutil.EncodeBig(wei), jsonHex(t, got))
}

func TestRequest_liveEthBlockNumber(t *testing.T) {
	p := provider.New(connectLive(t), provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_blockNumber"})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(jsonHex(t, got), "0x"))
}

func TestRequest_liveSendTransaction(t *testing.T) {
	p := provider.New(connectLive(t), provider.Options{Confirm: &stubConfirm{approve: true}})
	got, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"from":  wallets.Addresses()[0],
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}},
	})
	require.NoError(t, err)
	hash, ok := got.(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(hash, "0x"))
	require.Len(t, hash, 66)
}

func jsonHex(t *testing.T, v any) string {
	t.Helper()
	switch x := v.(type) {
	case string:
		return x
	case json.RawMessage:
		var s string
		require.NoError(t, json.Unmarshal(x, &s))
		return s
	default:
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var s string
		require.NoError(t, json.Unmarshal(b, &s), "got %#v", v)
		return s
	}
}
