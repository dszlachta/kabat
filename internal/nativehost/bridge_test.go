package nativehost_test

import (
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/nativehost"
	"github.com/stretchr/testify/require"
)

func TestBridge_returnsWhenStdinCloses(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	_, stdoutW := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- nativehost.Bridge(filepath.Join(t.TempDir(), "kabat.sock"), stdinR, stdoutW)
	}()
	require.NoError(t, stdinW.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("bridge hung after stdin close")
	}
}

func TestBridge_redial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s")
	lis, err := ipc.ListenUnix(path)
	if err != nil {
		t.Skip(err)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	t.Cleanup(func() {
		_ = stdinW.Close()
		_ = stdoutR.Close()
	})
	done := make(chan error, 1)
	go func() { done <- nativehost.Bridge(path, stdinR, stdoutW) }()

	c1 := acceptUnix(t, lis)
	_, err = stdinW.Write([]byte("ping"))
	require.NoError(t, err)
	got := make([]byte, 4)
	require.NoError(t, c1.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = io.ReadFull(c1, got)
	require.NoError(t, err)
	require.Equal(t, "ping", string(got))

	require.NoError(t, c1.Close())
	require.Equal(t, "accountsChanged", readEventName(t, stdoutR))
	require.Equal(t, "disconnect", readEventName(t, stdoutR))

	c2 := acceptUnix(t, lis)
	_, err = stdinW.Write([]byte("pong"))
	require.NoError(t, err)
	got = make([]byte, 4)
	require.NoError(t, c2.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, err = io.ReadFull(c2, got)
	require.NoError(t, err)
	require.Equal(t, "pong", string(got))

	require.NoError(t, stdinW.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("bridge hung after stdin close")
	}
}

func acceptUnix(t *testing.T, lis net.Listener) net.Conn {
	t.Helper()
	type result struct {
		c   net.Conn
		err error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := lis.Accept()
		ch <- result{c, err}
	}()
	select {
	case r := <-ch:
		require.NoError(t, r.err)
		return r.c
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for native host dial")
		return nil
	}
}

func readEventName(t *testing.T, r io.Reader) string {
	t.Helper()
	type result struct {
		env ipc.Envelope
		err error
	}
	ch := make(chan result, 1)
	go func() {
		env, err := ipc.ReadEnvelope(r)
		ch <- result{env, err}
	}()
	var rslt result
	select {
	case rslt = <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout reading event")
	}
	require.NoError(t, rslt.err)
	var payload ipc.ResultPayload
	require.NoError(t, json.Unmarshal(rslt.env.Payload, &payload))
	return payload.Event
}
