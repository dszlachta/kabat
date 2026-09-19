package walletservice_test

import (
	"bytes"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestRequest_sendTransaction_attachesEIP712Schema(t *testing.T) {
	addr, svc := loadCallContractABI(t, &ethAPI{}, "eip712_abi.json")
	from := wallets.Addresses()[0]

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			pending <- data
		}
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"from": from,
			"to":   addr.Hex(),
			"data": "0x",
		}})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case raw := <-pending:
		view, ok := raw.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "Withdraw", view["primaryType"])
		fields, ok := view["fields"].([]any)
		require.True(t, ok)
		require.GreaterOrEqual(t, len(fields), 2)
		names := fieldNames(t, fields)
		require.Contains(t, names, "EIP712Domain")
		require.Contains(t, names, "Withdraw")
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestRequest_sendTransaction_eip712ValuesFromCalldata(t *testing.T) {
	addr, svc := loadCallContractABI(t, &ethAPI{}, "eip712_abi.json")
	from := wallets.Addresses()[0]
	data := packWithdraw(t, big.NewInt(10))

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			pending <- data
		}
	})

	go func() {
		_, _ = svc.Request("eth_sendTransaction", []any{map[string]any{
			"from": from,
			"to":   addr.Hex(),
			"data": data,
		}})
	}()

	select {
	case raw := <-pending:
		view, ok := raw.(map[string]any)
		require.True(t, ok)
		fields, ok := view["fields"].([]any)
		require.True(t, ok)
		require.Equal(t, "10", nestedFieldText(t, fields, "Withdraw", "paymentId"))
		require.Equal(t, addr.Hex(), nestedFieldText(t, fields, "EIP712Domain", "verifyingContract"))
		require.Equal(t, "31337", nestedFieldText(t, fields, "EIP712Domain", "chainId"))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())
}

func TestRequest_sendTransaction_noEIP712WithoutABI(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	from := wallets.Addresses()[0]

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			pending <- data
		}
	})

	go func() {
		_, _ = svc.Request("eth_sendTransaction", []any{map[string]any{
			"from": from,
			"to":   addr.Hex(),
			"data": "0x",
		}})
	}()

	select {
	case raw := <-pending:
		view, ok := raw.(map[string]any)
		require.True(t, ok)
		_, hasFields := view["fields"]
		require.False(t, hasFields)
		_, hasPrimary := view["primaryType"]
		require.False(t, hasPrimary)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())
}

func fieldNames(t *testing.T, fields []any) []string {
	t.Helper()
	var names []string
	for _, item := range fields {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		name, _ := m["name"].(string)
		names = append(names, name)
	}
	return names
}

func nestedFieldText(t *testing.T, fields []any, typeName, fieldName string) string {
	t.Helper()
	for _, item := range fields {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		if m["name"] != typeName {
			continue
		}
		children, _ := m["fields"].([]any)
		for _, child := range children {
			cm, ok := child.(map[string]any)
			require.True(t, ok)
			if cm["name"] == fieldName {
				text, _ := cm["text"].(string)
				return text
			}
		}
	}
	t.Fatalf("missing %s.%s", typeName, fieldName)
	return ""
}

func packWithdraw(t *testing.T, paymentID *big.Int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "contracts", "testdata", "eip712_abi.json"))
	require.NoError(t, err)
	spec, err := abi.JSON(bytes.NewReader(raw))
	require.NoError(t, err)
	method := spec.Methods["withdraw"]
	packed, err := method.Inputs.Pack(paymentID, []byte{0xaa})
	require.NoError(t, err)
	return hexutil.Encode(append(method.ID, packed...))
}
