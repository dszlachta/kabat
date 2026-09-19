package mcpserver_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/mcpserver"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

type web3API struct {
	version string
}

func (a *web3API) ClientVersion() string {
	return a.version
}

type ethAPI struct {
	chainID    *big.Int
	codes      map[common.Address][]byte
	callResult hexutil.Bytes
	callBySel  map[string]hexutil.Bytes
}

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *ethAPI) GetBalance(context.Context, common.Address, rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (a *ethAPI) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func (a *ethAPI) Call(msg map[string]any, _ any) (hexutil.Bytes, error) {
	if a.callBySel != nil {
		data := ""
		if s, ok := msg["input"].(string); ok {
			data = s
		} else if s, ok := msg["data"].(string); ok {
			data = s
		}
		if len(data) >= 10 {
			if out, ok := a.callBySel[strings.ToLower(data[:10])]; ok {
				return out, nil
			}
		}
	}
	return a.callResult, nil
}

func (a *ethAPI) GetTransactionCount(common.Address, rpc.BlockNumberOrHash) hexutil.Uint64 {
	return 0
}

func (a *ethAPI) EstimateGas(map[string]any) hexutil.Uint64 {
	return 21000
}

func (a *ethAPI) GasPrice() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(1e9))
}

func (a *ethAPI) SendRawTransaction(data hexutil.Bytes) common.Hash {
	tx := new(types.Transaction)
	if err := tx.UnmarshalBinary(data); err != nil {
		return common.Hash{}
	}
	return tx.Hash()
}

func startAnvil(t *testing.T, eth *ethAPI) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("web3", &web3API{version: "anvil/v1.0.0"}))
	if eth == nil {
		eth = &ethAPI{chainID: big.NewInt(31337)}
	}
	if eth.chainID == nil {
		eth.chainID = big.NewInt(31337)
	}
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func mcpService(t *testing.T, eth *ethAPI) (*walletservice.WalletService, *mcp.ClientSession) {
	t.Helper()
	rpcSrv := startAnvil(t, eth)
	svc := walletservice.NewWithURL(rpcSrv.URL)
	ts := httptest.NewServer(mcpserver.Handler(svc))
	t.Cleanup(ts.Close)
	session := mcpSession(t, ts.URL)
	t.Cleanup(func() { _ = session.Close() })
	return svc, session
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	require.NoError(t, err)
	if res.IsError {
		t.Fatalf("%s: %s", name, toolText(t, res))
	}
	return res
}

func toolJSON(t *testing.T, res *mcp.CallToolResult, dest any) {
	t.Helper()
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(b, dest), string(b))
		return
	}
	require.NoError(t, json.Unmarshal([]byte(toolText(t, res)), dest), toolText(t, res))
}

func TestHandler_listAccounts(t *testing.T) {
	_, session := mcpService(t, nil)
	res := callTool(t, session, "list_accounts", map[string]any{})
	var out struct {
		Accounts []walletservice.Account `json:"accounts"`
	}
	toolJSON(t, res, &out)
	require.Len(t, out.Accounts, 10)
	require.Equal(t, wallets.Addresses()[0], out.Accounts[0].Address)
}

func TestHandler_sendEth(t *testing.T) {
	svc, session := mcpService(t, nil)
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})
	res := callTool(t, session, "send_eth", map[string]any{
		"to":    wallets.Addresses()[1],
		"value": "0x1",
	})
	var out struct {
		Hash string `json:"hash"`
	}
	toolJSON(t, res, &out)
	require.True(t, strings.HasPrefix(out.Hash, "0x"))
	require.Len(t, out.Hash, 66)
}

func TestHandler_sendERC20(t *testing.T) {
	token := common.HexToAddress("0x1111111111111111111111111111111111111111")
	code := append(append([]byte{0x60, 0x00}, []byte{0xa9, 0x05, 0x9c, 0xbb}...), []byte{0x70, 0xa0, 0x82, 0x31}...)
	svc, session := mcpService(t, &ethAPI{
		codes: map[common.Address][]byte{token: code},
		callBySel: map[string]hexutil.Bytes{
			"0x313ce567": common.LeftPadBytes([]byte{18}, 32),
			"0x70a08231": common.LeftPadBytes(new(big.Int).Exp(big.NewInt(10), big.NewInt(24), nil).Bytes(), 32),
		},
	})
	svc.SetEmitter(func(event string, data any) {
		t.Errorf("unexpected event %s", event)
	})
	res := callTool(t, session, "send_erc20", map[string]any{
		"token":  token.Hex(),
		"to":     wallets.Addresses()[1],
		"amount": "1",
	})
	var out struct {
		Hash string `json:"hash"`
	}
	toolJSON(t, res, &out)
	require.True(t, strings.HasPrefix(out.Hash, "0x"))
	require.Len(t, out.Hash, 66)
}

func TestHandler_loadABIAndCallContract(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	packed := common.LeftPadBytes([]byte{42}, 32)
	_, session := mcpService(t, &ethAPI{
		codes:      map[common.Address][]byte{addr: {0xff}},
		callResult: packed,
	})
	abiPath, err := filepath.Abs(filepath.Join("..", "contracts", "testdata", "abi_members.json"))
	require.NoError(t, err)

	loaded := callTool(t, session, "load_contract_abi", map[string]any{
		"address": addr.Hex(),
		"path":    abiPath,
	})
	require.Contains(t, toolText(t, loaded), `"balanceOf"`)

	res := callTool(t, session, "call_contract", map[string]any{
		"address":  addr.Hex(),
		"selector": "0x70a08231",
		"args":     []string{"0x1111111111111111111111111111111111111111"},
	})
	var out walletservice.ContractCallResult
	toolJSON(t, res, &out)
	require.Equal(t, "view", out.Kind)
	require.Empty(t, out.Hash)
	require.Equal(t, []walletservice.TxArg{{Type: "uint256", Text: "42"}}, out.Outputs)
}

func TestHandler_confirmTransaction(t *testing.T) {
	svc, session := mcpService(t, nil)
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_sendTransaction", []any{map[string]any{
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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	callTool(t, session, "confirm_transaction", map[string]any{})
	select {
	case got := <-result:
		require.NoError(t, got.err)
		hash, ok := got.v.(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(hash, "0x"))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestHandler_cancelTransaction(t *testing.T) {
	svc, session := mcpService(t, nil)
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingTransaction" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	errc := make(chan error, 1)
	go func() {
		_, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}})
		errc <- err
	}()
	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	callTool(t, session, "cancel_transaction", map[string]any{})
	select {
	case err := <-errc:
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4001, rpcErr.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestHandler_allowConnection(t *testing.T) {
	svc, session := mcpService(t, nil)
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingConnection" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	sessionULID := "01TESTSESSIONMCP000000000001"
	want := wallets.Addresses()[2]
	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method:      "eth_requestAccounts",
			Origin:      "https://dapp.example",
			SessionULID: sessionULID,
		})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()
	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	callTool(t, session, "allow_connection", map[string]any{"address": want})
	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, []string{want}, got.v)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}

	accs, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: sessionULID,
	})
	require.NoError(t, err)
	require.Equal(t, []string{want}, accs)
}

func TestHandler_denyConnection(t *testing.T) {
	svc, session := mcpService(t, nil)
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingConnection" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	errc := make(chan error, 1)
	go func() {
		_, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method:      "eth_requestAccounts",
			Origin:      "https://dapp.example",
			SessionULID: "01TESTSESSIONMCP000000000002",
		})
		errc <- err
	}()
	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	callTool(t, session, "deny_connection", map[string]any{})
	select {
	case err := <-errc:
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4001, rpcErr.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestHandler_dappMessages(t *testing.T) {
	svc, session := mcpService(t, nil)
	_, err := svc.RequestFromDapp(ipc.RequestPayload{
		ULID:        "01TESTMESSAGEMCP00000000000",
		Method:      "eth_chainId",
		Params:      []any{},
		SessionULID: "01TESTSESSIONMCP000000000003",
	})
	require.Error(t, err)

	res := callTool(t, session, "dapp_messages", map[string]any{})
	var out struct {
		Messages []walletservice.DappMessage `json:"messages"`
	}
	toolJSON(t, res, &out)
	require.NotEmpty(t, out.Messages)
	require.Equal(t, "eth_chainId", out.Messages[0].Method)
	require.Equal(t, "01TESTMESSAGEMCP00000000000", out.Messages[0].ULID)
}
