//go:build integration

package walletservice_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
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
