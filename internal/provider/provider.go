package provider

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	codeUnsupportedMethod = 4200
	codeDisconnected      = 4900
)

// RequestArguments is the EIP-1193 request payload.
type RequestArguments struct {
	Method string
	Params any
}

// ProviderRpcError is the EIP-1193 error shape.
type ProviderRpcError struct {
	Code    int
	Message string
	Data    any
}

func (e *ProviderRpcError) Error() string {
	return e.Message
}

// Options configures a Provider.
type Options struct {
	Selected common.Address
	Confirm  Confirmer
}

// Provider is an EIP-1193 Ethereum provider backed by a local Anvil client.
type Provider struct {
	chain    *chain.Client
	selected common.Address
	confirm  Confirmer

	mu        sync.RWMutex
	listeners map[string][]func(any)
}

// New wraps an Anvil client. A nil client is treated as disconnected.
func New(c *chain.Client, opts Options) *Provider {
	selected := opts.Selected
	if selected == (common.Address{}) {
		if accs := wallets.Accounts(); len(accs) > 0 {
			selected = accs[0].Address
		}
	}
	return &Provider{
		chain:     c,
		selected:  selected,
		confirm:   opts.Confirm,
		listeners: make(map[string][]func(any)),
	}
}

// Request handles an EIP-1193 RPC request.
func (p *Provider) Request(ctx context.Context, args RequestArguments) (any, error) {
	if p.chain == nil {
		err := &ProviderRpcError{Code: codeDisconnected, Message: "provider is disconnected"}
		p.emit("disconnect", err)
		return nil, err
	}

	switch args.Method {
	case "eth_accounts", "eth_requestAccounts":
		return p.accountList(), nil
	case "eth_chainId":
		id, err := p.chain.ChainID(ctx)
		if err != nil {
			return nil, wrapError(err)
		}
		return hexutil.EncodeBig(id), nil
	case "eth_sendTransaction":
		return p.sendTransaction(ctx, args.Params)
	case "eth_sign", "personal_sign", "eth_signTypedData", "eth_signTypedData_v4":
		return nil, &ProviderRpcError{
			Code:    codeUnsupportedMethod,
			Message: "the method " + args.Method + " is not supported",
		}
	}

	var result any
	err := p.chain.RPC().CallContext(ctx, &result, args.Method, rpcArgs(args.Params)...)
	if err != nil {
		return nil, wrapError(err)
	}
	return result, nil
}

func (p *Provider) accountList() []string {
	selected := p.selected.Hex()
	out := []string{selected}
	for _, addr := range wallets.Addresses() {
		if addr != selected {
			out = append(out, addr)
		}
	}
	return out
}

// On subscribes to an EIP-1193 event.
func (p *Provider) On(event string, handler func(any)) {
	if handler == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listeners[event] = append(p.listeners[event], handler)
}

// RemoveListener unsubscribes a previously registered handler.
func (p *Provider) RemoveListener(event string, handler func(any)) {
	if handler == nil {
		return
	}
	id := reflect.ValueOf(handler).Pointer()
	p.mu.Lock()
	defer p.mu.Unlock()
	handlers := p.listeners[event]
	kept := handlers[:0]
	for _, h := range handlers {
		if reflect.ValueOf(h).Pointer() != id {
			kept = append(kept, h)
		}
	}
	p.listeners[event] = kept
}

func (p *Provider) emit(event string, data any) {
	p.mu.RLock()
	handlers := append([]func(any){}, p.listeners[event]...)
	p.mu.RUnlock()
	for _, h := range handlers {
		h(data)
	}
}

func rpcArgs(params any) []any {
	if params == nil {
		return nil
	}
	switch p := params.(type) {
	case []any:
		return p
	default:
		return []any{p}
	}
}

func wrapError(err error) error {
	out := &ProviderRpcError{Code: codeDisconnected, Message: err.Error()}
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		out.Code = rpcErr.ErrorCode()
		out.Message = rpcErr.Error()
	}
	var dataErr rpc.DataError
	if errors.As(err, &dataErr) {
		out.Data = dataErr.ErrorData()
	}
	return out
}
