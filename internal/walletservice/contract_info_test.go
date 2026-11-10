package walletservice_test

import (
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestContractInfo_unknownEmptyABI(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	addr := "0x5555555555555555555555555555555555555555"
	got := svc.ContractInfo(addr)
	require.Equal(t, common.HexToAddress(addr).Hex(), got.Address)
	require.Empty(t, got.ABI)
	require.Empty(t, got.Events)
	require.Empty(t, got.Functions)
}

func TestLoadContractABI_cancel(t *testing.T) {
	addr := common.HexToAddress("0x6666666666666666666666666666666666666666")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0x60, 0x00}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	svc.SetPickJSONFile(func() (string, error) { return "", nil })

	got, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	require.Empty(t, got.ABI)
}

func TestLoadContractABI_loadsJSON(t *testing.T) {
	addr := common.HexToAddress("0x7777777777777777777777777777777777777777")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	abiPath := filepath.Join("..", "contracts", "testdata", "abi.json")
	svc.SetPickJSONFile(func() (string, error) { return abiPath, nil })

	got, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	require.Contains(t, got.ABI, `"name"`)
	require.Equal(t, got.ABI, svc.ContractInfo(addr.Hex()).ABI)
}

func TestLoadContractABI_invalidFile(t *testing.T) {
	addr := common.HexToAddress("0x8888888888888888888888888888888888888888")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	bad := filepath.Join(t.TempDir(), "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte("{"), 0o600))
	svc.SetPickJSONFile(func() (string, error) { return bad, nil })

	_, err := svc.LoadContractABI(addr.Hex())
	require.Error(t, err)
}

func TestLoadContractABI_members(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0x9999999999999999999999999999999999999999")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	abiPath := filepath.Join("..", "contracts", "testdata", "abi_members.json")
	svc.SetPickJSONFile(func() (string, error) { return abiPath, nil })

	got, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	require.Equal(t, []walletservice.ContractEvent{{
		Name: "Transfer",
		Inputs: []walletservice.ContractArg{
			{Type: "address", Name: "from", Indexed: true},
			{Type: "address", Name: "to", Indexed: true},
			{Type: "uint256", Name: "value"},
		},
	}}, got.Events)
	require.Equal(t, []walletservice.ContractFunction{
		{
			StateMutability: "view",
			Name:            "balanceOf",
			Selector:        "0x70a08231",
			Inputs:          []walletservice.ContractArg{{Type: "address", Name: "account"}},
			Outputs:         []walletservice.ContractArg{{Type: "uint256"}},
		},
		{
			StateMutability: "nonpayable",
			Name:            "transfer",
			Selector:        "0xa9059cbb",
			Inputs: []walletservice.ContractArg{
				{Type: "address", Name: "to"},
				{Type: "uint256", Name: "amount"},
			},
			Outputs: []walletservice.ContractArg{{Type: "bool"}},
		},
	}, got.Functions)
}

func TestLoadContractABI_eip712(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	srv := startAnvil(t, &ethAPI{
		codes: map[common.Address][]byte{addr: {0xff}},
	})
	svc := walletservice.NewWithURL(srv.URL)
	abiPath := filepath.Join("..", "contracts", "testdata", "eip712_abi.json")
	svc.SetPickJSONFile(func() (string, error) { return abiPath, nil })

	got, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	require.NotNil(t, got.EIP712)
	require.Equal(t, "Withdraw", got.EIP712.PrimaryType)
	require.Equal(t, []walletservice.EIP712Field{
		{Name: "paymentId", Type: "uint256"},
		{Name: "recipient", Type: "address"},
		{Name: "action", Type: "uint8"},
	}, got.EIP712.Types["Withdraw"])
	require.Equal(t, got.EIP712, svc.ContractInfo(addr.Hex()).EIP712)
}

func loadCallContract(t *testing.T, eth *ethAPI) (common.Address, *walletservice.WalletService) {
	t.Helper()
	return loadCallContractABI(t, eth, "abi_members.json")
}

func loadCallContractABI(t *testing.T, eth *ethAPI, abiFile string) (common.Address, *walletservice.WalletService) {
	t.Helper()
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if eth.codes == nil {
		eth.codes = map[common.Address][]byte{}
	}
	eth.codes[addr] = []byte{0xff}
	if eth.chainID == nil {
		eth.chainID = big.NewInt(31337)
	}
	srv := startAnvil(t, eth)
	client, err := chain.ConnectURL(t.Context(), srv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	c, err := contracts.New(t.Context(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("..", "contracts", "testdata", abiFile)))
	return addr, walletservice.NewWithURL(srv.URL)
}

func TestCallContract_viewUnpacksUint(t *testing.T) {
	packed := common.LeftPadBytes([]byte{42}, 32)
	addr, svc := loadCallContract(t, &ethAPI{callResult: packed})
	from := wallets.Accounts()[1].Address.Hex()

	got, err := svc.CallContract(addr.Hex(), "0x70a08231", []string{
		"0x1111111111111111111111111111111111111111",
	}, from, "")
	require.NoError(t, err)
	require.Equal(t, "view", got.Kind)
	require.Equal(t, from, got.From)
	require.Empty(t, got.Hash)
	require.Equal(t, []walletservice.TxArg{{
		Type: "uint256",
		Text: "42",
	}}, got.Outputs)
}

func TestCallContractAuto_viewSkipsConfirm(t *testing.T) {
	packed := common.LeftPadBytes([]byte{42}, 32)
	addr, svc := loadCallContract(t, &ethAPI{callResult: packed})
	from := wallets.Accounts()[1].Address.Hex()
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})

	got, err := svc.CallContractAuto(addr.Hex(), "0x70a08231", []string{
		"0x1111111111111111111111111111111111111111",
	}, from, "")
	require.NoError(t, err)
	require.Equal(t, "view", got.Kind)
	require.Equal(t, from, got.From)
	require.Empty(t, got.Hash)
	require.Equal(t, []walletservice.TxArg{{
		Type: "uint256",
		Text: "42",
	}}, got.Outputs)
}

func TestCallContractAuto_sendSkipsConfirm(t *testing.T) {
	addr, svc := loadCallContract(t, &ethAPI{})
	from := wallets.Accounts()[2].Address.Hex()
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})

	got, err := svc.CallContractAuto(addr.Hex(), "0xa9059cbb", []string{
		"0x1111111111111111111111111111111111111111",
		"1",
	}, from, "")
	require.NoError(t, err)
	require.Equal(t, "send", got.Kind)
	require.Equal(t, from, got.From)
	require.Empty(t, got.Outputs)
	require.True(t, strings.HasPrefix(got.Hash, "0x"))
	require.Len(t, got.Hash, 66)
}

func TestCallContract_sendUsesConfirmer(t *testing.T) {
	addr, svc := loadCallContract(t, &ethAPI{})
	from := wallets.Accounts()[2].Address.Hex()

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		require.Equal(t, "pendingTransaction", event)
		pending <- data
	})

	result := make(chan struct {
		v   walletservice.ContractCallResult
		err error
	}, 1)
	go func() {
		v, err := svc.CallContract(addr.Hex(), "0xa9059cbb", []string{
			"0x1111111111111111111111111111111111111111",
			"1",
		}, from, "")
		result <- struct {
			v   walletservice.ContractCallResult
			err error
		}{v, err}
	}()

	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, "send", got.v.Kind)
		require.Equal(t, from, got.v.From)
		require.Empty(t, got.v.Outputs)
		require.True(t, strings.HasPrefix(got.v.Hash, "0x"))
		require.Len(t, got.v.Hash, 66)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for call result")
	}
}

func TestCallContract_unknownSelector(t *testing.T) {
	addr, svc := loadCallContract(t, &ethAPI{})
	_, err := svc.CallContract(addr.Hex(), "0xdeadbeef", nil, "", "")
	require.Error(t, err)
}

func TestCallContract_unknownFrom(t *testing.T) {
	addr, svc := loadCallContract(t, &ethAPI{})
	_, err := svc.CallContract(addr.Hex(), "0x70a08231", []string{
		"0x1111111111111111111111111111111111111111",
	}, "0x0000000000000000000000000000000000000001", "")
	require.ErrorIs(t, err, walletservice.ErrUnknownAccount)
}

func TestCallContract_emptyFromUsesSelected(t *testing.T) {
	packed := common.LeftPadBytes([]byte{7}, 32)
	addr, svc := loadCallContract(t, &ethAPI{callResult: packed})
	got, err := svc.CallContract(addr.Hex(), "0x70a08231", []string{
		"0x1111111111111111111111111111111111111111",
	}, "", "")
	require.NoError(t, err)
	require.Equal(t, "view", got.Kind)
	require.Equal(t, svc.Selected(), got.From)
}

func TestCallContract_payableSendsValue(t *testing.T) {
	addr, svc := loadCallContractABI(t, &ethAPI{}, "abi_payable.json")
	from := wallets.Accounts()[1].Address.Hex()
	wei := new(big.Int).SetUint64(1_000_000_000_000_000_000)
	want := hexutil.EncodeBig(wei)

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			pending <- data
		}
	})

	result := make(chan struct {
		v   walletservice.ContractCallResult
		err error
	}, 1)
	go func() {
		v, err := svc.CallContract(addr.Hex(), selectorNamed(t, svc, "deposit"), nil, from, want)
		result <- struct {
			v   walletservice.ContractCallResult
			err error
		}{v, err}
	}()

	select {
	case data := <-pending:
		raw, ok := data.(map[string]any)
		require.True(t, ok)
		require.Equal(t, want, raw["value"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, "send", got.v.Kind)
		require.True(t, strings.HasPrefix(got.v.Hash, "0x"))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for call result")
	}
}

func TestCallContract_nonPayableRejectsValue(t *testing.T) {
	addr, svc := loadCallContract(t, &ethAPI{})
	_, err := svc.CallContract(addr.Hex(), "0xa9059cbb", []string{
		"0x1111111111111111111111111111111111111111",
		"1",
	}, "", "0x1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "value not allowed")
}

func selectorNamed(t *testing.T, svc *walletservice.WalletService, name string) string {
	t.Helper()
	for id, n := range svc.ABISelectors() {
		if n == name {
			return id
		}
	}
	t.Fatalf("no selector for %s", name)
	return ""
}
