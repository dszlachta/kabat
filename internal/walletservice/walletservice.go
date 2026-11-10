package walletservice

import (
	"context"
	"errors"
	"log"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/config"
	"github.com/dszlachta/kabat/internal/ipc"
	"github.com/dszlachta/kabat/internal/jsonfmt"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

var ErrUnknownAccount = errors.New("unknown account")
var ErrNoPendingTransaction = errors.New("no pending transaction")
var ErrPendingTransaction = errors.New("another transaction is pending")
var ErrNoPendingConnection = errors.New("no pending connection")
var ErrPendingConnection = errors.New("another connection is pending")
var ErrUnknownConnection = errors.New("unknown connection")
var ErrInvalidAnvilURL = errors.New("invalid Anvil URL")

const (
	codeUserRejected = 4001
	codeUnauthorized = 4100
	maxDappMessages  = 200
)

// DappMessage is one inbound dapp RPC or the matching wallet reply.
type DappMessage struct {
	Direction   string `json:"direction"`
	SessionULID string `json:"sessionUlid"`
	ULID        string `json:"ulid"`
	Method      string `json:"method"`
	Params      any    `json:"params,omitempty"`
	Result      any    `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
	PayloadJSON string `json:"payloadJson,omitempty"`
}

// DappConnection is a dapp session allowed to use a wallet.
type DappConnection struct {
	Origin      string `json:"origin"`
	SessionULID string `json:"sessionUlid"`
}

// Account is a predefined local wallet as shown in the UI.
type Account struct {
	Address     string           `json:"address"`
	Name        string           `json:"name"`
	Balance     string           `json:"balance"`
	Selected    bool             `json:"selected"`
	Connected   int              `json:"connected"`
	Connections []DappConnection `json:"connections"`
}

// Settings is the persisted app preferences shown in the Settings window.
type Settings struct {
	AnvilURL        string `json:"anvilUrl"`
	DefaultAnvilURL string `json:"defaultAnvilUrl"`
	MCP             bool   `json:"mcp"`
	MCPURL          string `json:"mcpUrl"`
}

// WalletService exposes Anvil wallets to the Wails frontend.
type WalletService struct {
	url        string
	configPath string
	selected   common.Address

	mu            sync.Mutex
	emit          func(event string, data any)
	onSelected    func()
	pickJSONFile  func() (string, error)
	onMCP         func(enabled bool) error
	reply         chan bool
	connections   map[string]connection
	connectULID   string
	connectOrigin string
	connectCh     chan connectResult
	dappMessages  []DappMessage
	txs           []StoredTx
	txsByWallet   map[common.Address][]StoredTx
	txHashes      map[common.Hash]struct{}
	lastIngested  uint64
	names         map[common.Address]string
	abis          map[common.Address]string
	mcp           bool
}

type connection struct {
	origin string
	addr   common.Address
}

type connectResult struct {
	addr   common.Address
	reject bool
}

type uiConfirmer struct {
	s *WalletService
}

func (c uiConfirmer) Confirm(ctx context.Context, tx provider.TransactionRequest) (bool, error) {
	return c.s.waitConfirm(ctx, tx)
}

// New connects to the default Anvil endpoint and the user config file.
func New() *WalletService {
	return NewWithConfig(chain.DefaultURL, config.DefaultPath())
}

// NewWithURL connects to the given JSON-RPC URL without reading or writing config.
func NewWithURL(url string) *WalletService {
	return NewWithConfig(url, "")
}

// NewWithConfig connects to url and persists names, ABI paths, and settings at path.
// An empty path keeps names, ABI paths, and settings in memory only.
func NewWithConfig(url, path string) *WalletService {
	svc := &WalletService{
		url:         url,
		configPath:  path,
		connections: make(map[string]connection),
		txsByWallet: make(map[common.Address][]StoredTx),
		txHashes:    make(map[common.Hash]struct{}),
		names:       make(map[common.Address]string),
		abis:        make(map[common.Address]string),
	}
	if accs := wallets.Accounts(); len(accs) > 0 {
		svc.selected = accs[0].Address
	}
	svc.loadConfig()
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

// SetPickJSONFile registers the native JSON file picker (empty path = cancel).
//
//wails:ignore
func (s *WalletService) SetPickJSONFile(fn func() (string, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pickJSONFile = fn
}

// SetOnMCP registers a callback invoked when the MCP toggle changes.
//
//wails:ignore
func (s *WalletService) SetOnMCP(fn func(enabled bool) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMCP = fn
}

// Selected returns the checksummed address of the selected account.
func (s *WalletService) Selected() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selected.Hex()
}

// AddRandomWallet generates a new key, adds it to the list, and selects it.
func (s *WalletService) AddRandomWallet() error {
	acc, err := wallets.AddRandom()
	if err != nil {
		return err
	}
	return s.Select(acc.Address.Hex())
}

// AddWalletFromKey imports a hex private key (optional 0x), adds it, and selects it.
func (s *WalletService) AddWalletFromKey(privateKey string) error {
	acc, err := wallets.AddPrivateKey(privateKey)
	if err != nil {
		return err
	}
	return s.Select(acc.Address.Hex())
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
	s.emitAccounts()
	return nil
}

// SetAccountName stores a display name for a known wallet and writes config.
func (s *WalletService) SetAccountName(address, name string) error {
	addr := common.HexToAddress(address)
	if _, ok := wallets.Find(addr); !ok {
		return ErrUnknownAccount
	}
	s.mu.Lock()
	if s.names == nil {
		s.names = make(map[common.Address]string)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		delete(s.names, addr)
	} else {
		s.names[addr] = name
	}
	s.mu.Unlock()
	s.saveConfig()
	s.emitAccounts()
	return nil
}

// Settings returns the current Anvil URL and MCP toggle.
func (s *WalletService) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	url := s.url
	if url == "" {
		url = chain.DefaultURL
	}
	return Settings{
		AnvilURL:        url,
		DefaultAnvilURL: chain.DefaultURL,
		MCP:             s.mcp,
		MCPURL:          config.MCPURL(),
	}
}

// SetAnvilURL stores the JSON-RPC URL (empty resets to the default) and writes config.
func (s *WalletService) SetAnvilURL(raw string) error {
	next, err := normalizeAnvilURL(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.url = next
	s.mu.Unlock()
	s.persistSettings()
	s.emitAccounts()
	return nil
}

// SetMCP stores the MCP server toggle, starts or stops the server, and writes config.
func (s *WalletService) SetMCP(enabled bool) error {
	s.mu.Lock()
	prev := s.mcp
	s.mcp = enabled
	onMCP := s.onMCP
	s.mu.Unlock()
	if onMCP != nil {
		if err := onMCP(enabled); err != nil {
			s.mu.Lock()
			s.mcp = prev
			s.mu.Unlock()
			return err
		}
	}
	s.persistSettings()
	return nil
}

func normalizeAnvilURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return chain.DefaultURL, nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", ErrInvalidAnvilURL
	}
	return raw, nil
}

func (s *WalletService) persistSettings() {
	s.mu.Lock()
	path := s.configPath
	anvil := s.url
	mcp := s.mcp
	s.mu.Unlock()
	if path == "" {
		return
	}
	if anvil == chain.DefaultURL {
		anvil = ""
	}
	if err := config.Update(path, func(f *config.File) {
		f.AnvilURL = anvil
		f.MCP = mcp
	}); err != nil {
		log.Print(err)
	}
}

func (s *WalletService) loadConfig() {
	if s.configPath == "" {
		return
	}
	f, err := config.Load(s.configPath)
	if err != nil {
		log.Print(err)
		return
	}
	for hexAddr, w := range f.Wallets {
		if w.Name == "" {
			continue
		}
		s.names[common.HexToAddress(hexAddr)] = w.Name
	}
	for hexAddr, c := range f.Contracts {
		if c.ABI == "" {
			continue
		}
		s.abis[common.HexToAddress(hexAddr)] = c.ABI
	}
	if u := strings.TrimSpace(f.AnvilURL); u != "" {
		s.url = u
	}
	s.mcp = f.MCP
}

func (s *WalletService) saveConfig() {
	s.mu.Lock()
	path := s.configPath
	names := maps.Clone(s.names)
	abis := maps.Clone(s.abis)
	s.mu.Unlock()
	if path == "" {
		return
	}
	wallets := map[string]config.Wallet{}
	for addr, name := range names {
		if name == "" {
			continue
		}
		wallets[addr.Hex()] = config.Wallet{Name: name}
	}
	contracts := map[string]config.Contract{}
	for addr, abiPath := range abis {
		if abiPath == "" {
			continue
		}
		contracts[addr.Hex()] = config.Contract{ABI: abiPath}
	}
	if len(wallets) == 0 {
		wallets = nil
	}
	if len(contracts) == 0 {
		contracts = nil
	}
	if err := config.Update(path, func(f *config.File) {
		f.Wallets = wallets
		f.Contracts = contracts
	}); err != nil {
		log.Print(err)
	}
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
	names := make(map[common.Address]string, len(s.names))
	for addr, name := range s.names {
		names[addr] = name
	}
	byAddr := make(map[common.Address][]DappConnection, len(s.connections))
	for session, c := range s.connections {
		byAddr[c.addr] = append(byAddr[c.addr], DappConnection{
			Origin:      c.origin,
			SessionULID: session,
		})
	}
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
		balance := "0x0"
		if bals[i] != nil {
			balance = hexutil.EncodeBig(bals[i])
		}
		conns := byAddr[w.Address]
		if conns == nil {
			conns = []DappConnection{}
		}
		out = append(out, Account{
			Address:     w.Address.Hex(),
			Name:        names[w.Address],
			Balance:     balance,
			Selected:    w.Address == selected,
			Connected:   len(conns),
			Connections: conns,
		})
	}
	return out, nil
}

// Request handles an EIP-1193 RPC request using the selected account and UI confirmer.
func (s *WalletService) Request(method string, params any) (any, error) {
	s.mu.Lock()
	selected := s.selected
	s.mu.Unlock()
	return s.requestWith(selected, method, params)
}

// RequestFromDapp handles an extension RPC, routing by session ULID.
//
//wails:ignore
func (s *WalletService) RequestFromDapp(req ipc.RequestPayload) (result any, err error) {
	if req.Method == "kabat_disconnect" {
		disErr := s.Disconnect(req.SessionULID)
		if disErr != nil && !errors.Is(disErr, ErrUnknownConnection) {
			return nil, disErr
		}
		return nil, nil
	}

	s.logDappMessage(DappMessage{
		Direction:   "in",
		SessionULID: req.SessionULID,
		ULID:        req.ULID,
		Method:      req.Method,
		Params:      req.Params,
	})
	defer func() {
		out := DappMessage{
			Direction:   "out",
			SessionULID: req.SessionULID,
			ULID:        req.ULID,
			Method:      req.Method,
		}
		if err != nil {
			out.Error = err.Error()
		} else {
			out.Result = result
		}
		s.logDappMessage(out)
	}()

	if req.Method == "eth_requestAccounts" && req.Origin != "" {
		addr, connectErr := s.waitConnect(context.Background(), req.Origin, req.SessionULID)
		if connectErr != nil {
			return nil, connectErr
		}
		return []string{addr.Hex()}, nil
	}

	s.mu.Lock()
	conn, ok := s.connections[req.SessionULID]
	s.mu.Unlock()
	if !ok {
		if req.Method == "eth_accounts" {
			return []string{}, nil
		}
		return nil, &provider.ProviderRpcError{Code: codeUnauthorized, Message: "Unauthorized"}
	}
	if req.Method == "eth_accounts" {
		return []string{conn.addr.Hex()}, nil
	}
	return s.requestWith(conn.addr, req.Method, req.Params)
}

func dappPayloadJSON(msg DappMessage) string {
	if msg.Error != "" {
		return ""
	}
	if msg.Direction == "in" {
		if msg.Params == nil {
			return ""
		}
		return jsonfmt.Value(msg.Params)
	}
	if msg.Result == nil {
		return ""
	}
	return jsonfmt.Value(msg.Result)
}

func (s *WalletService) logDappMessage(msg DappMessage) {
	msg.PayloadJSON = dappPayloadJSON(msg)
	s.mu.Lock()
	s.dappMessages = append(s.dappMessages, msg)
	if len(s.dappMessages) > maxDappMessages {
		s.dappMessages = append([]DappMessage(nil), s.dappMessages[len(s.dappMessages)-maxDappMessages:]...)
	}
	out := slices.Clone(s.dappMessages)
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit("dappMessages", out)
	}
}

// DappMessages returns the in-memory dapp RPC log (newest last).
func (s *WalletService) DappMessages() []DappMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dappMessages == nil {
		return []DappMessage{}
	}
	return slices.Clone(s.dappMessages)
}

type approveAll struct{}

func (approveAll) Confirm(context.Context, provider.TransactionRequest) (bool, error) {
	return true, nil
}

func (s *WalletService) requestWith(selected common.Address, method string, params any) (any, error) {
	return s.requestWithConfirm(selected, method, params, uiConfirmer{s})
}

func (s *WalletService) requestWithConfirm(selected common.Address, method string, params any, confirm provider.Confirmer) (any, error) {
	ctx := context.Background()
	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return nil, chain.ErrAnvilUnavailable
	}
	defer client.Close()

	p := provider.New(client, provider.Options{
		Selected: selected,
		Confirm:  confirm,
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

	raw := s.confirmPayload(ctx, tx)
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

func (s *WalletService) waitConnect(ctx context.Context, origin, sessionULID string) (common.Address, error) {
	s.mu.Lock()
	if s.connectCh != nil {
		s.mu.Unlock()
		return common.Address{}, ErrPendingConnection
	}
	reply := make(chan connectResult, 1)
	s.connectCh = reply
	s.connectULID = sessionULID
	s.connectOrigin = origin
	emit := s.emit
	s.mu.Unlock()

	if emit != nil {
		emit("pendingConnection", map[string]any{
			"origin":      origin,
			"sessionUlid": sessionULID,
			"accounts":    wallets.Addresses(),
		})
	}

	select {
	case res := <-reply:
		if res.reject {
			return common.Address{}, &provider.ProviderRpcError{Code: codeUserRejected, Message: "User Rejected Request"}
		}
		return res.addr, nil
	case <-ctx.Done():
		s.mu.Lock()
		if s.connectCh == reply {
			s.connectCh = nil
			s.connectULID = ""
			s.connectOrigin = ""
		}
		s.mu.Unlock()
		return common.Address{}, ctx.Err()
	}
}

// AllowConnect approves the pending dapp connection for address.
func (s *WalletService) AllowConnect(address string) error {
	addr := common.HexToAddress(address)
	if _, ok := wallets.Find(addr); !ok {
		return ErrUnknownAccount
	}
	s.mu.Lock()
	if s.connectCh == nil {
		s.mu.Unlock()
		return ErrNoPendingConnection
	}
	if s.connectULID != "" {
		s.connections[s.connectULID] = connection{origin: s.connectOrigin, addr: addr}
	}
	s.connectCh <- connectResult{addr: addr}
	s.connectCh = nil
	s.connectULID = ""
	s.connectOrigin = ""
	s.mu.Unlock()
	s.emitAccounts()
	return nil
}

func (s *WalletService) emitAccounts() {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit == nil {
		return
	}
	accs, err := s.Accounts()
	if err != nil {
		return
	}
	emit("accounts", accs)
}

// CancelConnect rejects the pending dapp connection.
func (s *WalletService) CancelConnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connectCh == nil {
		return ErrNoPendingConnection
	}
	s.connectCh <- connectResult{reject: true}
	s.connectCh = nil
	s.connectULID = ""
	s.connectOrigin = ""
	return nil
}

// Disconnect drops a dapp session so later RPCs from it are unauthorized.
func (s *WalletService) Disconnect(sessionULID string) error {
	s.mu.Lock()
	if _, ok := s.connections[sessionULID]; !ok {
		s.mu.Unlock()
		return ErrUnknownConnection
	}
	delete(s.connections, sessionULID)
	s.mu.Unlock()
	s.emitAccounts()
	return nil
}
