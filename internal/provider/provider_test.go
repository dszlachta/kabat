package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
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
	chainID     *big.Int
	blockNumber hexutil.Uint64
	balances    map[common.Address]*big.Int
	nonce       hexutil.Uint64
	gas         hexutil.Uint64
	gasPrice    *big.Int
	sent        []hexutil.Bytes
	sendErr     error
}

type rpcDataErr struct {
	msg  string
	code int
	data any
}

func (e *rpcDataErr) Error() string          { return e.msg }
func (e *rpcDataErr) ErrorCode() int         { return e.code }
func (e *rpcDataErr) ErrorData() interface{} { return e.data }

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *ethAPI) BlockNumber() hexutil.Uint64 {
	return a.blockNumber
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

func (a *ethAPI) GetTransactionCount(common.Address, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return a.nonce
}

func (a *ethAPI) EstimateGas(map[string]any) hexutil.Uint64 {
	if a.gas == 0 {
		return 21000
	}
	return a.gas
}

func (a *ethAPI) GasPrice() *hexutil.Big {
	if a.gasPrice == nil {
		return (*hexutil.Big)(big.NewInt(1e9))
	}
	return (*hexutil.Big)(a.gasPrice)
}

func (a *ethAPI) SendRawTransaction(data hexutil.Bytes) (common.Hash, error) {
	if a.sendErr != nil {
		return common.Hash{}, a.sendErr
	}
	a.sent = append(a.sent, append(hexutil.Bytes{}, data...))
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}

type stubConfirm struct {
	approve bool
	err     error
	got     provider.TransactionRequest
}

func (s *stubConfirm) Confirm(_ context.Context, tx provider.TransactionRequest) (bool, error) {
	s.got = tx
	return s.approve, s.err
}

func connectAnvil(t *testing.T, eth *ethAPI) *chain.Client {
	t.Helper()
	if eth == nil {
		eth = &ethAPI{chainID: big.NewInt(31337), blockNumber: 123}
	}
	if eth.chainID == nil {
		eth.chainID = big.NewInt(31337)
	}
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("web3", &web3API{version: "anvil/v1.0.0"}))
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	client, err := chain.ConnectURL(context.Background(), httpSrv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func newProvider(t *testing.T, eth *ethAPI, opts provider.Options) *provider.Provider {
	t.Helper()
	return provider.New(connectAnvil(t, eth), opts)
}

func TestRequest_ethAccounts(t *testing.T) {
	p := newProvider(t, nil, provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_accounts"})
	require.NoError(t, err)
	require.Equal(t, wallets.Addresses(), got)
}

func TestRequest_ethAccounts_selectedFirst(t *testing.T) {
	selected := wallets.Accounts()[3].Address
	p := newProvider(t, nil, provider.Options{Selected: selected})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_accounts"})
	require.NoError(t, err)
	list, ok := got.([]string)
	require.True(t, ok)
	require.Equal(t, selected.Hex(), list[0])
	require.Len(t, list, 10)
	require.ElementsMatch(t, wallets.Addresses(), list)
}

func TestRequest_ethRequestAccounts(t *testing.T) {
	p := newProvider(t, nil, provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_requestAccounts"})
	require.NoError(t, err)
	require.Equal(t, wallets.Addresses(), got)
}

func TestRequest_ethChainId(t *testing.T) {
	p := newProvider(t, &ethAPI{chainID: big.NewInt(31337)}, provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_chainId"})
	require.NoError(t, err)
	require.Equal(t, "0x7a69", got)
}

func TestRequest_disconnected(t *testing.T) {
	p := provider.New(nil, provider.Options{})
	_, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_chainId"})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 4900, rpcErr.Code)
}

func TestRequest_unsupportedWalletMethod(t *testing.T) {
	p := newProvider(t, nil, provider.Options{})
	for _, method := range []string{"eth_sign", "personal_sign", "eth_signTypedData", "eth_signTypedData_v4"} {
		_, err := p.Request(context.Background(), provider.RequestArguments{Method: method})
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4200, rpcErr.Code, method)
	}
}

func TestRequest_sendTransaction_rejected(t *testing.T) {
	p := newProvider(t, &ethAPI{chainID: big.NewInt(31337)}, provider.Options{
		Confirm: &stubConfirm{approve: false},
	})
	_, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}},
	})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 4001, rpcErr.Code)
}

func TestRequest_sendTransaction_approved(t *testing.T) {
	eth := &ethAPI{chainID: big.NewInt(31337), nonce: 7, gas: 21000, gasPrice: big.NewInt(1e9)}
	confirm := &stubConfirm{approve: true}
	p := newProvider(t, eth, provider.Options{Confirm: confirm})
	to := wallets.Addresses()[1]
	got, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"from":  wallets.Addresses()[0],
			"to":    to,
			"value": "0x0",
		}},
	})
	require.NoError(t, err)
	hash, ok := got.(string)
	require.True(t, ok)
	require.True(t, strings.HasPrefix(hash, "0x"))
	require.Len(t, hash, 66)
	require.Len(t, eth.sent, 1)
	require.Equal(t, wallets.Accounts()[0].Address, confirm.got.From)
	require.NotNil(t, confirm.got.Raw)
}

func TestRequest_sendTransaction_rpcErrorData(t *testing.T) {
	eth := &ethAPI{
		chainID:  big.NewInt(31337),
		nonce:    7,
		gas:      21000,
		gasPrice: big.NewInt(1e9),
		sendErr: &rpcDataErr{
			msg:  "execution reverted: insufficient funds",
			code: -32000,
			data: map[string]any{"reason": "insufficient funds"},
		},
	}
	p := newProvider(t, eth, provider.Options{Confirm: &stubConfirm{approve: true}})
	_, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"from":  wallets.Addresses()[0],
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}},
	})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, -32000, rpcErr.Code)
	require.Equal(t, "execution reverted: insufficient funds", rpcErr.Message)
	require.Equal(t, map[string]any{"reason": "insufficient funds"}, rpcErr.Data)
}

func TestRequest_sendTransaction_fromMismatch(t *testing.T) {
	p := newProvider(t, &ethAPI{chainID: big.NewInt(31337)}, provider.Options{
		Selected: wallets.Accounts()[0].Address,
		Confirm:  &stubConfirm{approve: true},
	})
	_, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"from": wallets.Addresses()[1],
			"to":   wallets.Addresses()[2],
		}},
	})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 4100, rpcErr.Code)
}

func TestRequest_sendTransaction_unknownFrom(t *testing.T) {
	p := newProvider(t, &ethAPI{chainID: big.NewInt(31337)}, provider.Options{
		Confirm: &stubConfirm{approve: true},
	})
	_, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_sendTransaction",
		Params: []any{map[string]any{
			"from": "0x0000000000000000000000000000000000000001",
			"to":   wallets.Addresses()[1],
		}},
	})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 4100, rpcErr.Code)
}

func TestRequest_forwardsEthMethods(t *testing.T) {
	p := newProvider(t, &ethAPI{
		chainID:     big.NewInt(31337),
		blockNumber: 123,
	}, provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_blockNumber"})
	require.NoError(t, err)
	require.Equal(t, "0x7b", jsonString(t, got))
}

func TestRequest_forwardsEthGetBalance(t *testing.T) {
	addr := wallets.Accounts()[0].Address
	want := big.NewInt(42)
	p := newProvider(t, &ethAPI{
		chainID:  big.NewInt(31337),
		balances: map[common.Address]*big.Int{addr: want},
	}, provider.Options{})
	got, err := p.Request(context.Background(), provider.RequestArguments{
		Method: "eth_getBalance",
		Params: []any{addr.Hex(), "latest"},
	})
	require.NoError(t, err)
	require.Equal(t, hexutil.EncodeBig(want), jsonString(t, got))
}

func TestOnAndRemoveListener(t *testing.T) {
	p := provider.New(nil, provider.Options{})
	var calls int
	h := func(any) { calls++ }
	p.On("disconnect", h)
	_, err := p.Request(context.Background(), provider.RequestArguments{Method: "eth_accounts"})
	require.Error(t, err)
	require.Equal(t, 1, calls)

	p.RemoveListener("disconnect", h)
	_, err = p.Request(context.Background(), provider.RequestArguments{Method: "eth_accounts"})
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func jsonString(t *testing.T, v any) string {
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

func TestProviderRpcError_isError(t *testing.T) {
	err := &provider.ProviderRpcError{Code: 4200, Message: "unsupported"}
	require.EqualError(t, err, "unsupported")
	require.True(t, errors.As(err, new(*provider.ProviderRpcError)))
}
