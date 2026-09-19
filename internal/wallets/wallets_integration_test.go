//go:build integration

package wallets_test

import (
	"context"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestAddresses_matchLiveAnvilAccounts(t *testing.T) {
	client, err := chain.Connect(context.Background())
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	t.Cleanup(client.Close)

	var raw []string
	err = client.RPC().CallContext(context.Background(), &raw, "eth_accounts")
	require.NoError(t, err)
	got := make([]string, len(raw))
	for i, addr := range raw {
		got[i] = common.HexToAddress(addr).Hex()
	}
	require.Equal(t, wallets.Addresses(), got)
}
