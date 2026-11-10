package walletservice

import (
	"context"
	"errors"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestParseUnits(t *testing.T) {
	got, err := parseUnits("1", 18)
	require.NoError(t, err)
	require.Equal(t, new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil), got)

	got, err = parseUnits("1.5", 18)
	require.NoError(t, err)
	want := new(big.Int).Mul(big.NewInt(15), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil))
	require.Equal(t, want, got)

	got, err = parseUnits("0.000000000000000001", 18)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(1), got)

	got, err = parseUnits("42", 0)
	require.NoError(t, err)
	require.Equal(t, big.NewInt(42), got)

	_, err = parseUnits("", 18)
	require.ErrorIs(t, err, errInvalidAmt)
	_, err = parseUnits("-1", 18)
	require.ErrorIs(t, err, errInvalidAmt)
	_, err = parseUnits("1.2.3", 18)
	require.ErrorIs(t, err, errInvalidAmt)
	_, err = parseUnits("1.123", 2)
	require.ErrorIs(t, err, errInvalidAmt)
}

func TestFormatUnits(t *testing.T) {
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	require.Equal(t, "1", formatUnits(base, 18))
	require.Equal(t, "0.000000000000000001", formatUnits(big.NewInt(1), 18))
	require.Equal(t, "0", formatUnits(big.NewInt(0), 18))
	require.Equal(t, "1.5", formatUnits(new(big.Int).Mul(big.NewInt(15), new(big.Int).Exp(big.NewInt(10), big.NewInt(17), nil)), 18))
	require.Equal(t, "42", formatUnits(big.NewInt(42), 0))
}

func TestProbeERC20_token(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{addr: erc20Bytecode()},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	addCreate(svc, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", addr)

	got, err := svc.Tokens()
	require.NoError(t, err)
	require.Equal(t, []ERC20Token{{
		Address:  addr.Hex(),
		Name:     "Wrapped Ether",
		Symbol:   "WETH",
		Decimals: 18,
		Balance:  "1000000",
	}}, got)
}

func TestProbeERC20_return42(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes:   map[common.Address][]byte{addr: erc20Bytecode()},
		callRaw: []byte{42},
	})
	svc := NewWithURL(srv.URL)
	addCreate(svc, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", addr)

	got, err := svc.Tokens()
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestProbeERC20_missingTransfer(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	code := append([]byte{0x60, 0x00}, selBalanceOf...)
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{addr: code},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	addCreate(svc, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", addr)

	got, err := svc.Tokens()
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestTokens_onlyERC20Creates(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	other := common.HexToAddress("0x2222222222222222222222222222222222222222")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{
			token: erc20Bytecode(),
			other: {0x60, 0x00, 0x60, 0x00, 0xfd},
		},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	addCreate(svc, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", other)
	addCreate(svc, "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", token)

	got, err := svc.Tokens()
	require.NoError(t, err)
	require.Equal(t, []ERC20Token{{
		Address:  token.Hex(),
		Name:     "Wrapped Ether",
		Symbol:   "WETH",
		Decimals: 18,
		Balance:  "1000000",
	}}, got)
}

func TestContractDetails_erc20(t *testing.T) {
	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{addr: erc20Bytecode()},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	hash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	addCreate(svc, hash, addr)

	got, err := svc.ContractDetails(addr.Hex())
	require.NoError(t, err)
	require.NotNil(t, got.ERC20)
	require.Equal(t, "Wrapped Ether", got.ERC20.Name)
	require.Equal(t, "WETH", got.ERC20.Symbol)
	require.Equal(t, uint8(18), got.ERC20.Decimals)
	require.Equal(t, "0.000000000000000001", got.ERC20.TotalSupply)
	require.Equal(t, hash, got.Hash)
}

func TestSendERC20_confirms(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{token: erc20Bytecode()},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	from := wallets.Addresses()[0]
	to := wallets.Addresses()[1]
	require.NoError(t, svc.Select(from))

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
		hash, err := svc.SendERC20(token.Hex(), to, "1")
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
		require.Equal(t, token.Hex(), raw["to"])
		dataHex, _ := raw["data"].(string)
		require.True(t, strings.HasPrefix(strings.ToLower(dataHex), "0xa9059cbb"))
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

func TestSendERC20Auto_skipsConfirmDialog(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{token: erc20Bytecode()},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	from := wallets.Addresses()[1]
	to := wallets.Addresses()[2]
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})
	hash, err := svc.SendERC20Auto(from, token.Hex(), to, "1")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "0x"))
	require.Len(t, hash, 66)
}

func TestSendERC20_rejectsInvalidTo(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	_, err := svc.SendERC20("0x1111111111111111111111111111111111111111", "", "1")
	require.Error(t, err)
	_, err = svc.SendERC20("0x1111111111111111111111111111111111111111", "not-an-address", "1")
	require.Error(t, err)
}

func TestSendERC20_rejectsInvalidAmount(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{token: erc20Bytecode()},
		calls: standardTokenCalls(t),
	})
	svc := NewWithURL(srv.URL)
	to := wallets.Addresses()[1]
	_, err := svc.SendERC20(token.Hex(), to, "")
	require.Error(t, err)
	_, err = svc.SendERC20(token.Hex(), to, "xyz")
	require.Error(t, err)
}

func TestSendERC20_rejectsInvalidToken(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	_, err := svc.SendERC20("not-an-address", wallets.Addresses()[1], "1")
	require.ErrorIs(t, err, errInvalidToken)
}

func TestSendERC20_insufficientBalance(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	calls := standardTokenCalls(t)
	calls[methodSel("balanceOf")] = packOut(t, "balanceOf", big.NewInt(0))
	srv := erc20TestServer(t, &erc20API{
		codes: map[common.Address][]byte{token: erc20Bytecode()},
		calls: calls,
	})
	svc := NewWithURL(srv.URL)
	_, err := svc.SendERC20Auto(wallets.Addresses()[0], token.Hex(), wallets.Addresses()[1], "1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient WETH balance")
}

func erc20Bytecode() []byte {
	return append(append([]byte{0x60, 0x00}, selTransfer...), selBalanceOf...)
}

func methodSel(name string) string {
	return strings.ToLower(hexutil.Encode(erc20ABI.Methods[name].ID))
}

func packOut(t *testing.T, method string, args ...any) hexutil.Bytes {
	t.Helper()
	b, err := erc20ABI.Methods[method].Outputs.Pack(args...)
	require.NoError(t, err)
	return b
}

func standardTokenCalls(t *testing.T) map[string]hexutil.Bytes {
	t.Helper()
	return map[string]hexutil.Bytes{
		methodSel("decimals"):    packOut(t, "decimals", uint8(18)),
		methodSel("name"):        packOut(t, "name", "Wrapped Ether"),
		methodSel("symbol"):      packOut(t, "symbol", "WETH"),
		methodSel("totalSupply"): packOut(t, "totalSupply", big.NewInt(1)),
		methodSel("balanceOf"):   packOut(t, "balanceOf", new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil)),
	}
}

func addCreate(svc *WalletService, hash string, addr common.Address) {
	svc.addTx(StoredTx{
		Hash:        hash,
		BlockNumber: 1,
		From:        wallets.Accounts()[0].Address.Hex(),
		Kind:        "create",
		State:       "final",
		Ok:          true,
		Contract:    addr.Hex(),
	})
}

type erc20API struct {
	codes   map[common.Address][]byte
	calls   map[string]hexutil.Bytes
	callRaw hexutil.Bytes
}

func (erc20API) ClientVersion() string { return "anvil/v1.0.0" }

func (erc20API) ChainId() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(31337))
}

func (erc20API) BlockNumber() hexutil.Uint64 { return 0 }

func (a *erc20API) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func (erc20API) GetBalance(context.Context, common.Address, rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (a *erc20API) Call(msg map[string]any, _ any) (hexutil.Bytes, error) {
	if len(a.callRaw) > 0 {
		return a.callRaw, nil
	}
	data := callInput(msg)
	if len(data) >= 10 && a.calls != nil {
		if out, ok := a.calls[strings.ToLower(data[:10])]; ok {
			return out, nil
		}
	}
	return nil, errors.New("no data")
}

func callInput(msg map[string]any) string {
	for _, key := range []string{"input", "data"} {
		switch v := msg[key].(type) {
		case string:
			return v
		case []byte:
			return hexutil.Encode(v)
		case hexutil.Bytes:
			return v.String()
		}
	}
	return ""
}

func (erc20API) GetTransactionCount(common.Address, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return 0
}

func (erc20API) EstimateGas(map[string]any) hexutil.Uint64 {
	return 21000
}

func (erc20API) GasPrice() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1e9))
}

func (a *erc20API) SendRawTransaction(data hexutil.Bytes) common.Hash {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}
	}
	return tx.Hash()
}

func erc20TestServer(t *testing.T, api *erc20API) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	if api == nil {
		api = &erc20API{}
	}
	require.NoError(t, srv.RegisterName("web3", api))
	require.NoError(t, srv.RegisterName("eth", api))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}
