package ipc

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"

	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/report"
	"github.com/dszlachta/kabat/internal/wallets"
)

const (
	codeInvalidRequest = -32600
	codeDisconnected   = 4900
)

// Handler is the EIP-1193 request backend (WalletService).
type Handler interface {
	Request(method string, params any) (any, error)
	Selected() string
}

// Server accepts framed envelopes and dispatches Handler.Request.
type Server struct {
	handler Handler

	mu    sync.Mutex
	conns map[*client]struct{}
}

type client struct {
	conn net.Conn
	wmu  sync.Mutex
}

// NewServer wraps handler.
func NewServer(handler Handler) *Server {
	return &Server{handler: handler, conns: make(map[*client]struct{})}
}

// Serve accepts connections until the listener closes.
func (s *Server) Serve(lis net.Listener) error {
	for {
		c, err := lis.Accept()
		if err != nil {
			return err
		}
		go s.ServeConn(c)
	}
}

// ServeConn handles a single framed connection.
func (s *Server) ServeConn(nc net.Conn) {
	s.handle(nc)
}

func (s *Server) handle(nc net.Conn) {
	cl := &client{conn: nc}
	s.mu.Lock()
	s.conns[cl] = struct{}{}
	s.mu.Unlock()
	defer func() {
		// Connection is already finished; close error is not actionable.
		_ = nc.Close()
		s.mu.Lock()
		delete(s.conns, cl)
		s.mu.Unlock()
	}()

	s.pushConnect(cl)

	for {
		env, err := ReadEnvelope(nc)
		if err != nil {
			if errors.Is(err, ErrMissingULID) {
				id, idErr := NewULID()
				if idErr != nil {
					report.Error(idErr)
					return
				}
				s.reply(cl, id, ResultPayload{Error: &RPCError{Code: codeInvalidRequest, Message: ErrMissingULID.Error()}})
				continue
			}
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
				// Peer closed the connection.
				return
			}
			report.Error(err)
			return
		}
		req, err := ParseRequest(env)
		if err != nil {
			s.reply(cl, env.ULID, ResultPayload{Error: &RPCError{Code: codeInvalidRequest, Message: err.Error()}})
			continue
		}
		result, callErr := s.handler.Request(req.Method, req.Params)
		if callErr != nil {
			s.reply(cl, env.ULID, ResultPayload{Error: rpcError(callErr)})
			continue
		}
		s.reply(cl, env.ULID, ResultPayload{Result: result})
	}
}

func (s *Server) pushConnect(cl *client) {
	chainID, err := s.handler.Request("eth_chainId", nil)
	if err != nil {
		report.Error(err)
		return
	}
	env, err := Event("connect", map[string]any{"chainId": chainID})
	if err != nil {
		report.Error(err)
		return
	}
	if err := cl.write(env); err != nil {
		report.Error(err)
	}
}

// Broadcast sends an EIP-1193 event to every connected client.
func (s *Server) Broadcast(event string, params any) {
	env, err := Event(event, params)
	if err != nil {
		report.Error(err)
		return
	}
	s.mu.Lock()
	clients := make([]*client, 0, len(s.conns))
	for cl := range s.conns {
		clients = append(clients, cl)
	}
	s.mu.Unlock()
	for _, cl := range clients {
		if err := cl.write(env); err != nil {
			report.Error(err)
		}
	}
}

// AccountsChanged notifies clients of the current account list (selected first).
func (s *Server) AccountsChanged() {
	s.Broadcast("accountsChanged", orderedAccounts(s.handler.Selected()))
}

func (c *client) write(env Envelope) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return WriteFrame(c.conn, env)
}

func (s *Server) reply(cl *client, ulid string, payload ResultPayload) {
	env, err := Reply(ulid, payload)
	if err != nil {
		report.Error(err)
		return
	}
	if err := cl.write(env); err != nil {
		report.Error(err)
	}
}

func rpcError(err error) *RPCError {
	var rpcErr *provider.ProviderRpcError
	if errors.As(err, &rpcErr) {
		return &RPCError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	}
	return &RPCError{Code: codeDisconnected, Message: err.Error()}
}

func orderedAccounts(selected string) []string {
	out := []string{selected}
	for _, addr := range wallets.Addresses() {
		if addr != selected {
			out = append(out, addr)
		}
	}
	return out
}

// EncodeJSON is a test helper.
func EncodeJSON(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
