package walletservice_test

import (
	"context"
	"encoding/json"
	"math/big"
	"net"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
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
	chainID    *big.Int
	balances   map[common.Address]*big.Int
	codes      map[common.Address][]byte
	nonce      hexutil.Uint64
	gas        hexutil.Uint64
	gasPrice   *big.Int
	callResult hexutil.Bytes
}

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
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

func (a *ethAPI) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func (a *ethAPI) Call(map[string]any, any) (hexutil.Bytes, error) {
	return a.callResult, nil
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
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}

func TestSelect_defaultsToFirstAccount(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.Equal(t, wallets.Addresses()[0], svc.Selected())
}

func TestSelect_updatesSelected(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	want := wallets.Addresses()[2]
	require.NoError(t, svc.Select(want))
	require.Equal(t, want, svc.Selected())
}

func TestSelect_rejectsUnknown(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	err := svc.Select("0x0000000000000000000000000000000000000001")
	require.Error(t, err)
}

func TestSelect_notifiesOnSelected(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	var got string
	svc.SetOnSelected(func() { got = svc.Selected() })
	want := wallets.Addresses()[2]
	require.NoError(t, svc.Select(want))
	require.Equal(t, want, got)
}

func TestSelect_doesNotBroadcastAccountsChanged(t *testing.T) {
	httpSrv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(httpSrv.URL)
	ipcSrv := ipc.NewServer(svc)

	a, b := net.Pipe()
	t.Cleanup(func() {
		// Test teardown; the peer may already be closed.
		_ = a.Close()
		_ = b.Close()
	})
	go ipcSrv.ServeConn(a)

	require.NoError(t, b.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := ipc.ReadEnvelope(b) // connect
	require.NoError(t, err)
	_, err = ipc.ReadEnvelope(b) // chainChanged
	require.NoError(t, err)

	require.NoError(t, svc.Select(wallets.Addresses()[2]))
	require.NoError(t, b.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, err = ipc.ReadEnvelope(b)
	require.Error(t, err)
}

func TestWalletService_ipcHandler(t *testing.T) {
	var _ ipc.Handler = walletservice.NewWithURL("http://127.0.0.1:1")
}

func TestAccounts_returnsAddressesAndBalances(t *testing.T) {
	wallets.ResetAdded()
	ether := new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	balances := make(map[common.Address]*big.Int)
	for i, acc := range wallets.Accounts() {
		balances[acc.Address] = new(big.Int).Mul(ether, big.NewInt(int64(i+1)))
	}
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337), balances: balances})
	svc := walletservice.NewWithURL(srv.URL)

	got, err := svc.Accounts()
	require.NoError(t, err)
	require.Len(t, got, 10)
	for i, acc := range wallets.Accounts() {
		require.Equal(t, acc.Address.Hex(), got[i].Address)
		require.Equal(t, hexutil.EncodeBig(new(big.Int).Mul(ether, big.NewInt(int64(i+1)))), got[i].Balance)
		require.Equal(t, i == 0, got[i].Selected)
		require.Zero(t, got[i].Connected)
	}

	require.NoError(t, svc.Select(wallets.Addresses()[1]))
	got, err = svc.Accounts()
	require.NoError(t, err)
	require.False(t, got[0].Selected)
	require.True(t, got[1].Selected)
}

func TestSelect_emitsAccounts(t *testing.T) {
	wallets.ResetAdded()
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	got := make(chan []walletservice.Account, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "accounts" {
			accs, ok := data.([]walletservice.Account)
			require.True(t, ok)
			got <- accs
		}
	})
	want := wallets.Addresses()[2]
	require.NoError(t, svc.Select(want))
	select {
	case accs := <-got:
		require.Len(t, accs, 10)
		require.False(t, accs[0].Selected)
		require.True(t, accs[2].Selected)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for accounts")
	}
}

func TestAddRandomWallet_selectsAndEmits(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	got := make(chan []walletservice.Account, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "accounts" {
			accs, ok := data.([]walletservice.Account)
			require.True(t, ok)
			got <- accs
		}
	})
	require.NoError(t, svc.AddRandomWallet())
	select {
	case accs := <-got:
		require.Len(t, accs, 11)
		require.True(t, accs[10].Selected)
		require.Equal(t, svc.Selected(), accs[10].Address)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for accounts")
	}
}

func TestAddWalletFromKey_invalid(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	err := svc.AddWalletFromKey("not-a-key")
	require.ErrorIs(t, err, wallets.ErrInvalidPrivateKey)
	require.Len(t, wallets.Accounts(), 10)
}

func TestAddWalletFromKey_selectsImported(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	hexKey := "0x" + common.Bytes2Hex(crypto.FromECDSA(key))
	require.NoError(t, svc.AddWalletFromKey(hexKey))
	require.Equal(t, crypto.PubkeyToAddress(key.PublicKey).Hex(), svc.Selected())
	got, err := svc.Accounts()
	require.NoError(t, err)
	require.Len(t, got, 11)
	require.True(t, got[10].Selected)
}

func TestSetAccountName_roundTrip(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	addr := wallets.Addresses()[3]
	require.NoError(t, svc.SetAccountName(addr, "  Alice  "))
	got, err := svc.Accounts()
	require.NoError(t, err)
	require.Equal(t, "Alice", got[3].Name)
	require.Empty(t, got[0].Name)
}

func TestSetAccountName_unknown(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	err := svc.SetAccountName("0x0000000000000000000000000000000000000001", "Nope")
	require.ErrorIs(t, err, walletservice.ErrUnknownAccount)
}

func TestAccounts_mapsConnectionErrors(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.Accounts()
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func allowDapp(t *testing.T, svc *walletservice.WalletService, origin, session, address string) {
	t.Helper()
	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingConnection" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method:      "eth_requestAccounts",
			Origin:      origin,
			SessionULID: session,
		})
		done <- err
	}()
	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	require.NoError(t, svc.AllowConnect(address))
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestAccounts_connectedAfterAllow(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	one := wallets.Addresses()[2]
	two := wallets.Addresses()[3]

	allowDapp(t, svc, "https://dapp.example", "01TESTSESSION00000000000001", one)
	allowDapp(t, svc, "https://other.example", "01TESTSESSION00000000000002", one)
	allowDapp(t, svc, "https://third.example", "01TESTSESSION00000000000003", two)

	got, err := svc.Accounts()
	require.NoError(t, err)
	for _, acc := range got {
		switch acc.Address {
		case one:
			require.Equal(t, 2, acc.Connected)
			require.ElementsMatch(t, []walletservice.DappConnection{
				{Origin: "https://dapp.example", SessionULID: "01TESTSESSION00000000000001"},
				{Origin: "https://other.example", SessionULID: "01TESTSESSION00000000000002"},
			}, acc.Connections)
		case two:
			require.Equal(t, 1, acc.Connected)
			require.Equal(t, []walletservice.DappConnection{
				{Origin: "https://third.example", SessionULID: "01TESTSESSION00000000000003"},
			}, acc.Connections)
		default:
			require.Zero(t, acc.Connected)
			require.Empty(t, acc.Connections)
		}
	}
}

func TestDisconnect_dropsSession(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	addr := wallets.Addresses()[2]
	allowDapp(t, svc, "https://dapp.example", "01TESTSESSION00000000000001", addr)
	allowDapp(t, svc, "https://other.example", "01TESTSESSION00000000000002", addr)

	require.NoError(t, svc.Disconnect("01TESTSESSION00000000000001"))

	got, err := svc.Accounts()
	require.NoError(t, err)
	var acc walletservice.Account
	for _, a := range got {
		if a.Address == addr {
			acc = a
			break
		}
	}
	require.Equal(t, 1, acc.Connected)
	require.Equal(t, []walletservice.DappConnection{
		{Origin: "https://other.example", SessionULID: "01TESTSESSION00000000000002"},
	}, acc.Connections)

	empty, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: "01TESTSESSION00000000000001",
	})
	require.NoError(t, err)
	require.Equal(t, []string{}, empty)

	still, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: "01TESTSESSION00000000000002",
	})
	require.NoError(t, err)
	require.Equal(t, []string{addr}, still)
}

func TestDisconnect_unknownSession(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.ErrorIs(t, svc.Disconnect("01UNKNOWNSESSION00000000000"), walletservice.ErrUnknownConnection)
}

func TestRequestFromDapp_kabatDisconnect(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	addr := wallets.Addresses()[1]
	allowDapp(t, svc, "https://dapp.example", "01TESTSESSION00000000000001", addr)

	_, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "kabat_disconnect",
		SessionULID: "01TESTSESSION00000000000001",
	})
	require.NoError(t, err)

	empty, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: "01TESTSESSION00000000000001",
	})
	require.NoError(t, err)
	require.Equal(t, []string{}, empty)

	_, err = svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "kabat_disconnect",
		SessionULID: "01TESTSESSION00000000000001",
	})
	require.NoError(t, err)
}

func TestConfirmPending_withoutPending(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	require.ErrorIs(t, svc.ConfirmPending(), walletservice.ErrNoPendingTransaction)
	require.ErrorIs(t, svc.CancelPending(), walletservice.ErrNoPendingTransaction)
}

func TestRequest_mapsConnectionErrors(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	_, err := svc.Request("eth_chainId", nil)
	require.ErrorIs(t, err, chain.ErrAnvilUnavailable)
}

func TestRequest_sendTransactionConfirm(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		require.Equal(t, "pendingTransaction", event)
		pending <- data
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
	case raw := <-pending:
		view, ok := raw.(map[string]any)
		require.True(t, ok)
		pretty, ok := view["rawJSON"].(string)
		require.True(t, ok)
		require.Contains(t, pretty, "\n  ")
		require.NotContains(t, pretty, `"rawJSON"`)
	case <-time.After(2 * time.Second):
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
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestRequest_signTypedDataConfirm(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	from := wallets.Addresses()[0]

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		require.Equal(t, "pendingTransaction", event)
		pending <- data
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.Request("eth_signTypedData_v4", []any{from, map[string]any{
			"types": map[string]any{
				"EIP712Domain": []any{
					map[string]any{"name": "name", "type": "string"},
					map[string]any{"name": "version", "type": "string"},
					map[string]any{"name": "chainId", "type": "uint256"},
					map[string]any{"name": "verifyingContract", "type": "address"},
				},
				"Mail": []any{
					map[string]any{"name": "contents", "type": "string"},
				},
			},
			"primaryType": "Mail",
			"domain": map[string]any{
				"name":              "Ether Mail",
				"version":           "1",
				"chainId":           31337,
				"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC",
			},
			"message": map[string]any{"contents": "Hello, Bob!"},
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
		require.Equal(t, "typedData", view["kind"])
		b, err := json.Marshal(view)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(b, &decoded))
		fields, ok := decoded["fields"].([]any)
		require.True(t, ok)
		require.NotEmpty(t, fields)
		pretty, ok := view["rawJSON"].(string)
		require.True(t, ok)
		require.Contains(t, pretty, "\n  ")
		require.Contains(t, pretty, `"primaryType"`)
		require.NotContains(t, pretty, `"rawJSON"`)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-result:
		require.NoError(t, got.err)
		sig, ok := got.v.(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(sig, "0x"))
		require.Len(t, sig, 132)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestRequestFromDapp_allowConnectAndRoute(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	connected := wallets.Addresses()[2]
	require.NoError(t, svc.Select(wallets.Addresses()[0]))

	pending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "accounts" || event == "dappMessages" {
			return
		}
		require.Equal(t, "pendingConnection", event)
		pending <- data
	})

	result := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method:      "eth_requestAccounts",
			Origin:      "https://dapp.example/app",
			SessionULID: "01TESTSESSION00000000000000",
		})
		result <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case raw := <-pending:
		payload, ok := raw.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "https://dapp.example/app", payload["origin"])
		require.Equal(t, "01TESTSESSION00000000000000", payload["sessionUlid"])
		require.Equal(t, wallets.Addresses(), payload["accounts"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	require.NoError(t, svc.AllowConnect(connected))

	select {
	case got := <-result:
		require.NoError(t, got.err)
		require.Equal(t, []string{connected}, got.v)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}

	accs, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: "01TESTSESSION00000000000000",
	})
	require.NoError(t, err)
	require.Equal(t, []string{connected}, accs)

	txPending := make(chan any, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "dappMessages" {
			return
		}
		require.Equal(t, "pendingTransaction", event)
		txPending <- data
	})
	txResult := make(chan struct {
		v   any
		err error
	}, 1)
	go func() {
		v, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method: "eth_sendTransaction",
			Params: []any{map[string]any{
				"from":  connected,
				"to":    wallets.Addresses()[1],
				"value": "0x0",
			}},
			SessionULID: "01TESTSESSION00000000000000",
		})
		txResult <- struct {
			v   any
			err error
		}{v, err}
	}()

	select {
	case raw := <-txPending:
		tx, ok := raw.(map[string]any)
		require.True(t, ok)
		require.Equal(t, connected, tx["from"])
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.ConfirmPending())

	select {
	case got := <-txResult:
		require.NoError(t, got.err)
		hash, ok := got.v.(string)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(hash, "0x"))
		require.Len(t, hash, 66)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for sendTransaction result")
	}
}

func TestRequestFromDapp_unknownSession(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")

	accs, err := svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: "01UNKNOWNSESSION00000000000",
	})
	require.NoError(t, err)
	require.Equal(t, []string{}, accs)

	_, err = svc.RequestFromDapp(ipc.RequestPayload{
		Method:      "eth_chainId",
		SessionULID: "01UNKNOWNSESSION00000000000",
	})
	var rpcErr *provider.ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, 4100, rpcErr.Code)
}

func TestRequestFromDapp_cancelConnect(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	pending := make(chan struct{})
	svc.SetEmitter(func(event string, _ any) {
		if event == "pendingConnection" {
			select {
			case <-pending:
			default:
				close(pending)
			}
		}
	})

	result := make(chan error, 1)
	go func() {
		_, err := svc.RequestFromDapp(ipc.RequestPayload{
			Method:      "eth_requestAccounts",
			Origin:      "https://dapp.example",
			SessionULID: "01TESTSESSION00000000000000",
		})
		result <- err
	}()

	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	require.NoError(t, svc.CancelConnect())

	select {
	case err := <-result:
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4001, rpcErr.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestRequest_sendTransactionCancel(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)

	pending := make(chan struct{})
	svc.SetEmitter(func(string, any) { close(pending) })

	result := make(chan error, 1)
	go func() {
		_, err := svc.Request("eth_sendTransaction", []any{map[string]any{
			"to":    wallets.Addresses()[1],
			"value": "0x0",
		}})
		result <- err
	}()

	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingTransaction")
	}
	require.NoError(t, svc.CancelPending())

	select {
	case err := <-result:
		var rpcErr *provider.ProviderRpcError
		require.ErrorAs(t, err, &rpcErr)
		require.Equal(t, 4001, rpcErr.Code)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request result")
	}
}

func TestDappMessages_inAndOutShareULID(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	session := "01TESTSESSION00000000000001"
	allowDapp(t, svc, "https://dapp.example", session, wallets.Addresses()[2])

	const msgULID = "01TESTMESSAGE00000000000000"
	got, err := svc.RequestFromDapp(ipc.RequestPayload{
		ULID:        msgULID,
		Method:      "eth_chainId",
		Params:      []any{},
		SessionULID: session,
	})
	require.NoError(t, err)
	require.Equal(t, "0x7a69", got)

	var pair []walletservice.DappMessage
	for _, msg := range svc.DappMessages() {
		if msg.ULID == msgULID {
			pair = append(pair, msg)
		}
	}
	require.Len(t, pair, 2)
	require.Equal(t, "in", pair[0].Direction)
	require.Equal(t, "eth_chainId", pair[0].Method)
	require.Equal(t, session, pair[0].SessionULID)
	require.Equal(t, []any{}, pair[0].Params)
	require.Equal(t, "[]", pair[0].PayloadJSON)
	require.Equal(t, "out", pair[1].Direction)
	require.Equal(t, msgULID, pair[1].ULID)
	require.Equal(t, "eth_chainId", pair[1].Method)
	require.Equal(t, "0x7a69", pair[1].Result)
	require.Equal(t, `"0x7a69"`, pair[1].PayloadJSON)
}

func TestDappMessages_prettyJSON(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	session := "01TESTSESSION00000000000001"
	allowDapp(t, svc, "https://dapp.example", session, wallets.Addresses()[2])

	const msgULID = "01TESTMESSAGE00000000000001"
	_, err := svc.RequestFromDapp(ipc.RequestPayload{
		ULID:        msgULID,
		Method:      "eth_getBalance",
		Params:      []any{wallets.Addresses()[2], "latest"},
		SessionULID: session,
	})
	require.NoError(t, err)

	var in walletservice.DappMessage
	for _, msg := range svc.DappMessages() {
		if msg.ULID == msgULID && msg.Direction == "in" {
			in = msg
			break
		}
	}
	require.Equal(t, "eth_getBalance", in.Method)
	require.Contains(t, in.PayloadJSON, "\n  ")
	require.Contains(t, in.PayloadJSON, `"latest"`)
}

func TestDappMessages_RequestDoesNotLog(t *testing.T) {
	srv := startAnvil(t, &ethAPI{chainID: big.NewInt(31337)})
	svc := walletservice.NewWithURL(srv.URL)
	_, err := svc.Request("eth_chainId", nil)
	require.NoError(t, err)
	require.Empty(t, svc.DappMessages())
}
