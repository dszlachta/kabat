package mcpserver

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dszlachta/kabat/internal/config"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server is the loopback MCP HTTP server backed by WalletService.
type Server struct {
	svc *walletservice.WalletService

	mu      sync.Mutex
	httpSrv *http.Server
}

// New wraps svc.
func New(svc *walletservice.WalletService) *Server {
	return &Server{svc: svc}
}

// Handler is the streamable HTTP handler for tests and Start.
func Handler(svc *walletservice.WalletService) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "kabat", Version: "0.1.0"}, nil)
	addTools(server, svc)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
}

// Start listens on the configured loopback address.
func (s *Server) Start() error {
	return s.StartOn(config.MCPListenAddr)
}

// StartOn listens on addr (host:port).
func (s *Server) StartOn(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.httpSrv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: Handler(s.svc)}
	s.httpSrv = srv
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
		}
	}()
	return nil
}

// Stop closes the listener.
func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.httpSrv
	s.httpSrv = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

// SetEnabled starts the server when enabled is true and stops it otherwise.
func (s *Server) SetEnabled(enabled bool) error {
	if enabled {
		return s.Start()
	}
	s.mu.Lock()
	srv := s.httpSrv
	s.httpSrv = nil
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()
	return nil
}
