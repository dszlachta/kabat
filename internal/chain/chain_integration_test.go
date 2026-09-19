//go:build integration

package chain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/stretchr/testify/require"
)

func connectLive(t *testing.T) *chain.Client {
	t.Helper()
	client, err := chain.Connect(context.Background())
	require.NoError(t, err, "start Anvil on http://127.0.0.1:8545")
	t.Cleanup(client.Close)
	return client
}

func TestConnect_liveAnvil(t *testing.T) {
	client := connectLive(t)

	var version string
	err := client.RPC().CallContext(context.Background(), &version, "web3_clientVersion")
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(version), "anvil")

	id, err := client.ChainID(context.Background())
	require.NoError(t, err)
	require.Positive(t, id.Sign())

	bal, err := client.BalanceAt(context.Background(), wallets.Accounts()[0].Address)
	require.NoError(t, err)
	require.Positive(t, bal.Sign())
}
