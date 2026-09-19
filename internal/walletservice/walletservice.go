package walletservice

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

var ErrUnknownAccount = errors.New("unknown account")
var ErrNoPendingTransaction = errors.New("no pending transaction")
var ErrPendingTransaction = errors.New("another transaction is pending")

// Account is a predefined local wallet as shown in the UI.
type Account struct {
	Address  string `json:"address"`
	Balance  string `json:"balance"`
	Selected bool   `json:"selected"`
}

// WalletService exposes Anvil wallets to the Wails frontend.
type WalletService struct {
	url      string
	selected common.Address

	mu         sync.Mutex
	emit       func(event string, data any)
	onSelected func()
	reply      chan bool
}

type uiConfirmer struct {
	s *WalletService
}

func (c uiConfirmer) Confirm(ctx context.Context, tx provider.TransactionRequest) (bool, error) {
	return c.s.waitConfirm(ctx, tx)
}

// New connects to the default Anvil endpoint.
func New() *WalletService {
	return NewWithURL(chain.DefaultURL)
}

// NewWithURL connects to the given JSON-RPC URL.
func NewWithURL(url string) *WalletService {
	svc := &WalletService{url: url}
	if accs := wallets.Accounts(); len(accs) > 0 {
		svc.selected = accs[0].Address
	}
	return svc
}

// SetEmitter registers a callback used to notify the UI of a pending transaction.
//
//wails:ignore
func (s *WalletService) SetEmitter(emit func(event string, data any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit = emit
}

// SetOnSelected registers a callback invoked after the selected account changes.
//
//wails:ignore
func (s *WalletService) SetOnSelected(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onSelected = fn
}

// Selected returns the checksummed address of the selected account.
func (s *WalletService) Selected() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selected.Hex()
}

// Select sets the selected account used as the default signer.
func (s *WalletService) Select(address string) error {
	addr := common.HexToAddress(address)
	if _, ok := wallets.Find(addr); !ok {
		return ErrUnknownAccount
	}
	s.mu.Lock()
	s.selected = addr
	onSelected := s.onSelected
	s.mu.Unlock()
	if onSelected != nil {
		onSelected()
	}
	return nil
}

// Accounts returns the predefined Anvil wallets and their ETH balances.
func (s *WalletService) Accounts() ([]Account, error) {
	ctx := context.Background()
	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return nil, chain.ErrAnvilUnavailable
	}
	defer client.Close()

	s.mu.Lock()
	selected := s.selected
	s.mu.Unlock()

	accs := wallets.Accounts()
	addrs := make([]common.Address, len(accs))
	for i, w := range accs {
		addrs[i] = w.Address
	}
	bals, err := client.BalancesAt(ctx, addrs)
	if err != nil {
		return nil, chain.ErrAnvilUnavailable
	}

	out := make([]Account, 0, len(accs))
	for i, w := range accs {
		out = append(out, Account{
			Address:  w.Address.Hex(),
			Balance:  FormatEther(bals[i]),
			Selected: w.Address == selected,
		})
	}
	return out, nil
}

// Request handles an EIP-1193 RPC request using the selected account and UI confirmer.
func (s *WalletService) Request(method string, params any) (any, error) {
	ctx := context.Background()
	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return nil, chain.ErrAnvilUnavailable
	}
	defer client.Close()

	s.mu.Lock()
	selected := s.selected
	s.mu.Unlock()

	p := provider.New(client, provider.Options{
		Selected: selected,
		Confirm:  uiConfirmer{s},
	})
	return p.Request(ctx, provider.RequestArguments{Method: method, Params: params})
}

// ConfirmPending approves the current pending transaction.
func (s *WalletService) ConfirmPending() error {
	return s.resolvePending(true)
}

// CancelPending rejects the current pending transaction.
func (s *WalletService) CancelPending() error {
	return s.resolvePending(false)
}

func (s *WalletService) waitConfirm(ctx context.Context, tx provider.TransactionRequest) (bool, error) {
	s.mu.Lock()
	if s.reply != nil {
		s.mu.Unlock()
		return false, ErrPendingTransaction
	}
	reply := make(chan bool, 1)
	s.reply = reply
	emit := s.emit
	s.mu.Unlock()

	raw := tx.Raw
	if raw == nil {
		raw = map[string]any{}
	}
	if emit != nil {
		emit("pendingTransaction", raw)
	}

	select {
	case approved := <-reply:
		return approved, nil
	case <-ctx.Done():
		s.mu.Lock()
		if s.reply == reply {
			s.reply = nil
		}
		s.mu.Unlock()
		return false, ctx.Err()
	}
}

func (s *WalletService) resolvePending(approved bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reply == nil {
		return ErrNoPendingTransaction
	}
	s.reply <- approved
	s.reply = nil
	return nil
}

// FormatEther renders a wei amount as a trimmed ETH decimal string.
func FormatEther(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	s := new(big.Rat).SetFrac(wei, big.NewInt(params.Ether)).FloatString(18)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	return s
}
