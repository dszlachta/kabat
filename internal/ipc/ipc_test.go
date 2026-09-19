package ipc_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	require.NoError(t, b.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := ipc.ReadEnvelope(b) // connect
	require.NoError(t, err)
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

	require.NoError(t, b.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err := ipc.ReadEnvelope(b) // connect
	require.NoError(t, err)

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

func TestRelay_copiesBytes(t *testing.T) {
	sockA, sockB := net.Pipe()
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- ipc.Relay(sockA, stdinR, stdoutW) }()

	_, err := stdinW.Write([]byte("hello"))
	require.NoError(t, err)
	buf := make([]byte, 5)
	_, err = io.ReadFull(sockB, buf)
	require.NoError(t, err)
	require.Equal(t, "hello", string(buf))

	_, err = sockB.Write([]byte("world"))
	require.NoError(t, err)
	out := make([]byte, 5)
	_, err = io.ReadFull(stdoutR, out)
	require.NoError(t, err)
	require.Equal(t, "world", string(out))

	// Closing pipes unblocks Relay.
	_ = stdinW.Close()
	_ = sockB.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func mustULID(t *testing.T) string {
	t.Helper()
	id, err := ipc.NewULID()
	require.NoError(t, err)
	return id
}
