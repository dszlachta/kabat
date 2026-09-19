package walletservice_test

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestSendETH_usesSelectedAndConfirmer(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	from := wallets.Addresses()[1]
	to := wallets.Addresses()[2]
	require.NoError(t, svc.Select(from))
	wei := hexutil.EncodeBig(new(big.Int).SetUint64(1_000_000_000_000_000_000))

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			pending <- data
		}
	})

	result := make(chan struct {
		hash string
		err  error
	}, 1)
	go func() {
		hash, err := svc.SendETH(to, wei)
		result <- struct {
			hash string
			err  error
		}{hash, err}
	}()

	select {
	case data := <-pending:
		raw, ok := data.(map[string]any)
		require.True(t, ok)
		require.Equal(t, from, raw["from"])
		require.Equal(t, to, raw["to"])
		require.Equal(t, wei, raw["value"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.True(t, strings.HasPrefix(got.hash, "0x"))
		require.Len(t, got.hash, 66)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for send result")
	}
}

func TestSendETHAuto_skipsConfirmDialog(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	from := wallets.Addresses()[1]
	to := wallets.Addresses()[2]
	wei := hexutil.EncodeBig(new(big.Int).SetUint64(1_000_000_000_000_000_000))
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})
	hash, err := svc.SendETHAuto(from, to, wei)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "0x"))
	require.Len(t, hash, 66)
}

func TestSendETH_rejectsInvalidTo(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.SendETH("", "0x1")
	require.Error(t, err)
	_, err = svc.SendETH("not-an-address", "0x1")
	require.Error(t, err)
}

func TestSendETH_rejectsEmptyValue(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.SendETH(wallets.Addresses()[1], "")
	require.Error(t, err)
}

func TestSendETH_rejectsInvalidValue(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.SendETH(wallets.Addresses()[1], "xyz")
	require.Error(t, err)
}
