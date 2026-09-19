package ipc_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/stretchr/testify/require"
)

func TestEnvelope_roundTrip(t *testing.T) {
	id := mustULID(t)
	require.Len(t, id, 26)

	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_chainId"})
	require.NoError(t, err)
	env := ipc.Envelope{
		ULID:    id,
		TS:      ipc.NowMS(),
		Payload: payload,
	}
	raw, err := ipc.EncodeFrame(env)
	require.NoError(t, err)

	got, err := ipc.ReadEnvelope(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, env.ULID, got.ULID)
	req, err := ipc.ParseRequest(got)
	require.NoError(t, err)
	require.Equal(t, "eth_chainId", req.Method)
}

func TestDecodeEnvelope_missingULID(t *testing.T) {
	_, err := ipc.DecodeEnvelope([]byte(`{"ts":1,"payload":{"method":"eth_chainId"}}`))
	require.ErrorIs(t, err, ipc.ErrMissingULID)
}

func TestParseRequest_missingMethod(t *testing.T) {
	_, err := ipc.ParseRequest(ipc.Envelope{ULID: mustULID(t), Payload: []byte(`{"params":[]}`)})
	require.ErrorIs(t, err, ipc.ErrMissingMethod)
}

func TestParseRequest_keepsOriginAndSessionULID(t *testing.T) {
	req, err := ipc.ParseRequest(ipc.Envelope{
		ULID:    mustULID(t),
		Payload: []byte(`{"method":"eth_requestAccounts","origin":"https://dapp.example/app","sessionUlid":"01TESTSESSION00000000000000"}`),
	})
	require.NoError(t, err)
	require.Equal(t, "eth_requestAccounts", req.Method)
	require.Equal(t, "https://dapp.example/app", req.Origin)
	require.Equal(t, "01TESTSESSION00000000000000", req.SessionULID)
}

type mockHandler struct {
	selected string
	fn       func(method string, params any) (any, error)
}

func (m mockHandler) Selected() string { return m.selected }

func (m mockHandler) Request(method string, params any) (any, error) {
	return m.fn(method, params)
}

func startServer(t *testing.T, h ipc.Handler) net.Conn {
	t.Helper()
	srv := ipc.NewServer(h)
	a, b := net.Pipe()
	t.Cleanup(func() {
		// Test teardown; the peer may already be closed.
		_ = a.Close()
		_ = b.Close()
	})
	go srv.ServeConn(a)
	readEvent(t, b, "connect")
	readEvent(t, b, "chainChanged")
	return b
}

func TestListenUnix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kabat.sock")
	lis, err := ipc.ListenUnix(path)
	if err != nil {
		t.Skip(err)
	}
	require.NoError(t, lis.Close())
}

func readReply(t *testing.T, conn net.Conn) ipc.Envelope {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
	env, err := ipc.ReadEnvelope(conn)
	require.NoError(t, err)
	return env
}

func readEvent(t *testing.T, conn net.Conn, name string) ipc.ResultPayload {
	t.Helper()
	env := readReply(t, conn)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	require.Equal(t, name, payload.Event)
	return payload
}

func TestServer_requestResult(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			require.Equal(t, "eth_chainId", method)
			return "0x7a69", nil
		},
	}
	conn := startServer(t, h)
	id := mustULID(t)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_chainId"})
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(conn, ipc.Envelope{
		ULID:    id,
		TS:      ipc.NowMS(),
		Payload: payload,
	}))
	got := readReply(t, conn)
	require.Equal(t, id, got.ULID)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &result))
	require.Equal(t, "0x7a69", result.Result)
	require.Nil(t, result.Error)
}

func TestServer_rpcError(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			if method == "eth_chainId" {
				return "0x7a69", nil
			}
			return nil, &provider.ProviderRpcError{Code: 4001, Message: "User Rejected Request"}
		},
	}
	conn := startServer(t, h)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_sendTransaction", Params: []any{}})
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(conn, ipc.Envelope{
		ULID:    "01TESTREJECT00000000000000",
		TS:      1,
		Payload: payload,
	}))
	got := readReply(t, conn)
	require.Equal(t, "01TESTREJECT00000000000000", got.ULID)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &result))
	require.NotNil(t, result.Error)
	require.Equal(t, 4001, result.Error.Code)
}

func TestServer_rpcErrorMessageAndData(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			if method == "eth_chainId" {
				return "0x7a69", nil
			}
			return nil, &provider.ProviderRpcError{
				Code:    -32000,
				Message: "execution reverted",
				Data:    "0x08c379a0",
			}
		},
	}
	conn := startServer(t, h)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_sendTransaction"})
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(conn, ipc.Envelope{
		ULID:    mustULID(t),
		TS:      1,
		Payload: payload,
	}))
	got := readReply(t, conn)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &result))
	require.NotNil(t, result.Error)
	require.Equal(t, -32000, result.Error.Code)
	require.Equal(t, "execution reverted", result.Error.Message)
	require.Equal(t, "0x08c379a0", result.Error.Data)
}

func TestServer_unauthorized(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			if method == "eth_chainId" {
				return "0x7a69", nil
			}
			return nil, &provider.ProviderRpcError{Code: 4100, Message: "Unauthorized"}
		},
	}
	conn := startServer(t, h)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_sendTransaction"})
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(conn, ipc.Envelope{
		ULID:    mustULID(t),
		TS:      1,
		Payload: payload,
	}))
	got := readReply(t, conn)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &result))
	require.Equal(t, 4100, result.Error.Code)
}

func TestServer_missingULIDRejected(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			if method == "eth_chainId" {
				return "0x7a69", nil
			}
			return nil, errors.New("should not be called")
		},
	}
	conn := startServer(t, h)
	require.NoError(t, ipc.WriteFrame(conn, map[string]any{
		"ts":      1,
		"payload": map[string]any{"method": "eth_accounts"},
	}))
	got := readReply(t, conn)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &payload))
	require.NotNil(t, payload.Error)
	require.Equal(t, -32600, payload.Error.Code)
}

type dappHandler struct {
	mockHandler
	fromDapp func(ipc.RequestPayload) (any, error)
}

func (d dappHandler) RequestFromDapp(req ipc.RequestPayload) (any, error) {
	return d.fromDapp(req)
}

func TestServer_requestFromDapp(t *testing.T) {
	var got ipc.RequestPayload
	h := dappHandler{
		mockHandler: mockHandler{
			selected: wallets.Addresses()[0],
			fn: func(method string, params any) (any, error) {
				if method == "eth_chainId" {
					return "0x7a69", nil
				}
				return nil, errors.New("should use RequestFromDapp")
			},
		},
		fromDapp: func(req ipc.RequestPayload) (any, error) {
			got = req
			return []string{wallets.Addresses()[1]}, nil
		},
	}
	conn := startServer(t, h)
	id := mustULID(t)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{
		Method:      "eth_requestAccounts",
		Origin:      "https://dapp.example",
		SessionULID: "01TESTSESSION00000000000000",
	})
	require.NoError(t, err)
	require.NoError(t, ipc.WriteFrame(conn, ipc.Envelope{
		ULID:    id,
		TS:      ipc.NowMS(),
		Payload: payload,
	}))
	env := readReply(t, conn)
	require.Equal(t, id, env.ULID)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(env.Payload, &result))
	require.Equal(t, []any{wallets.Addresses()[1]}, result.Result)
	require.Equal(t, "eth_requestAccounts", got.Method)
	require.Equal(t, "https://dapp.example", got.Origin)
	require.Equal(t, "01TESTSESSION00000000000000", got.SessionULID)
	require.Equal(t, id, got.ULID)
}

func TestServer_broadcastAccountsChanged(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[2],
		fn: func(method string, params any) (any, error) {
			return "0x7a69", nil
		},
	}
	srv := ipc.NewServer(h)
	a, b := net.Pipe()
	t.Cleanup(func() {
		// Test teardown; the peer may already be closed.
		_ = a.Close()
		_ = b.Close()
	})
	go srv.ServeConn(a)

	readEvent(t, b, "connect")
	readEvent(t, b, "chainChanged")

	go srv.AccountsChanged()
	env, err := ipc.ReadEnvelope(b)
	require.NoError(t, err)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	require.Equal(t, "accountsChanged", payload.Event)
	list, ok := payload.Params.([]any)
	require.True(t, ok)
	require.Equal(t, wallets.Addresses()[2], list[0])
}

func TestServer_connectDoesNotWaitForChainId(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			switch method {
			case "eth_chainId":
				close(started)
				<-unblock
				return "0x7a69", nil
			case "eth_accounts":
				return []string{"0xabc"}, nil
			default:
				return nil, errors.New(method)
			}
		},
	}
	srv := ipc.NewServer(h)
	a, b := net.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	go srv.ServeConn(a)

	connectEnv := readReply(t, b)
	var connect ipc.ResultPayload
	require.NoError(t, json.Unmarshal(connectEnv.Payload, &connect))
	require.Equal(t, "connect", connect.Event)
	params, _ := connect.Params.(map[string]any)
	_, hasChainID := params["chainId"]
	require.False(t, hasChainID)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for eth_chainId")
	}

	id := mustULID(t)
	payload, err := ipc.EncodeJSON(ipc.RequestPayload{Method: "eth_accounts"})
	require.NoError(t, err)
	require.NoError(t, b.SetWriteDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, ipc.WriteFrame(b, ipc.Envelope{ULID: id, TS: 1, Payload: payload}))

	got := readReply(t, b)
	require.Equal(t, id, got.ULID)
	var result ipc.ResultPayload
	require.NoError(t, json.Unmarshal(got.Payload, &result))
	require.Equal(t, []any{"0xabc"}, result.Result)

	close(unblock)
	chainEnv := readReply(t, b)
	var chain ipc.ResultPayload
	require.NoError(t, json.Unmarshal(chainEnv.Payload, &chain))
	require.Equal(t, "chainChanged", chain.Event)
	require.Equal(t, "0x7a69", chain.Params)
}

func TestServer_connectEventWhenChainIdFails(t *testing.T) {
	h := mockHandler{
		selected: wallets.Addresses()[0],
		fn: func(method string, params any) (any, error) {
			return nil, errors.New("anvil down")
		},
	}
	srv := ipc.NewServer(h)
	a, b := net.Pipe()
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	go srv.ServeConn(a)

	env := readReply(t, b)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	require.Equal(t, "connect", payload.Event)
	params, _ := payload.Params.(map[string]any)
	_, hasChainID := params["chainId"]
	require.False(t, hasChainID)
}

func mustULID(t *testing.T) string {
	t.Helper()
	id, err := ipc.NewULID()
	require.NoError(t, err)
	return id
}
