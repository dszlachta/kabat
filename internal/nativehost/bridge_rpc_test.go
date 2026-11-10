package nativehost_test

import (
	"context"
	"encoding/json"
	"io"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/nativehost"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
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
	chainID *big.Int
}

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *ethAPI) GetBalance(context.Context, common.Address, rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	return (*hexutil.Big)(big.NewInt(0)), nil
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

func TestBridge_requestFromDapp(t *testing.T) {
	rpcSrv := rpc.NewServer()
	t.Cleanup(rpcSrv.Stop)
	require.NoError(t, rpcSrv.RegisterName("web3", &web3API{version: "anvil/v1.0.0"}))
	require.NoError(t, rpcSrv.RegisterName("eth", &ethAPI{chainID: big.NewInt(31337)}))
	httpSrv := httptest.NewServer(rpcSrv)
	t.Cleanup(httpSrv.Close)

	path := filepath.Join(t.TempDir(), "kabat.sock")
	lis, err := ipc.ListenUnix(path)
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	svc := walletservice.NewWithURL(httpSrv.URL)
	go func() { _ = ipc.NewServer(svc).Serve(lis) }()

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() {
		_ = stdinW.Close()
		_ = stdoutR.Close()
	})
	done := make(chan error, 1)
	go func() { done <- nativehost.Bridge(path, stdinR, stdoutW) }()

	require.Equal(t, "connect", readEventName(t, stdoutR))
	require.Equal(t, "chainChanged", readEventName(t, stdoutR))

	pending := make(chan struct{}, 1)
	svc.SetEmitter(func(event string, data any) {
		if event == "pendingConnection" {
			select {
			case pending <- struct{}{}:
			default:
			}
		}
	})

	sessionULID := "01TESTSESSIONBRIDGE00000001"
	want := wallets.Addresses()[2]
	reqID, err := ipc.NewULID()
	require.NoError(t, err)
	writeRequest(t, stdinW, reqID, ipc.RequestPayload{
		Method:      "eth_requestAccounts",
		Origin:      "https://dapp.example",
		SessionULID: sessionULID,
	})
	select {
	case <-pending:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for pendingConnection")
	}
	require.NoError(t, svc.AllowConnect(want))

	got := readResult(t, stdoutR)
	require.Equal(t, reqID, got.ulid)
	require.Nil(t, got.payload.Error)
	require.Equal(t, []any{want}, got.payload.Result)

	accID, err := ipc.NewULID()
	require.NoError(t, err)
	writeRequest(t, stdinW, accID, ipc.RequestPayload{
		Method:      "eth_accounts",
		SessionULID: sessionULID,
	})
	acc := readResult(t, stdoutR)
	require.Equal(t, accID, acc.ulid)
	require.Nil(t, acc.payload.Error)
	require.Equal(t, []any{want}, acc.payload.Result)

	require.NoError(t, stdinW.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("bridge hung after stdin close")
	}
}

func writeRequest(t *testing.T, w io.Writer, ulid string, req ipc.RequestPayload) {
	t.Helper()
	payload, err := ipc.EncodeJSON(req)
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(w, ipc.Envelope{
		ULID:    ulid,
		TS:      ipc.NowMS(),
		Payload: payload,
	}))
}

type framedResult struct {
	ulid    string
	payload ipc.ResultPayload
}

func readResult(t *testing.T, r io.Reader) framedResult {
	t.Helper()
	type outcome struct {
		env ipc.Envelope
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		env, err := ipc.ReadEnvelope(r)
		ch <- outcome{env, err}
	}()
	var got outcome
	select {
	case got = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading reply")
	}
	require.NoError(t, got.err)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.env.Payload, &payload))
	return framedResult{ulid: got.env.ULID, payload: payload}
}
