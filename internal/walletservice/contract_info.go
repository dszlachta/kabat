package walletservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ContractArg is one ABI event or function parameter.
type ContractArg struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Indexed bool   `json:"indexed"`
}

// ContractEvent is one ABI event for the Functions tab.
type ContractEvent struct {
	Name   string        `json:"name"`
	Inputs []ContractArg `json:"inputs"`
}

// ContractFunction is one ABI function for the Functions tab.
type ContractFunction struct {
	StateMutability string        `json:"stateMutability"`
	Name            string        `json:"name"`
	Selector        string        `json:"selector"`
	Inputs          []ContractArg `json:"inputs"`
	Outputs         []ContractArg `json:"outputs"`
}

// EIP712Field is one typed-data member.
type EIP712Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EIP712Info is the contract's EIP-712 schema from the ABI or artifact.
type EIP712Info struct {
	PrimaryType string                   `json:"primaryType,omitempty"`
	Types       map[string][]EIP712Field `json:"types,omitempty"`
}

// ContractInfo is the selected-contract pane payload.
type ContractInfo struct {
	Address   string             `json:"address"`
	Name      string             `json:"name"`
	ABI       string             `json:"abi"`
	Events    []ContractEvent    `json:"events"`
	Functions []ContractFunction `json:"functions"`
	EIP712    *EIP712Info        `json:"eip712,omitempty"`
}

var errNoFilePicker = errors.New("no file picker")

// ContractInfo returns in-memory details for address (empty ABI if unknown).
func (s *WalletService) ContractInfo(address string) ContractInfo {
	addr := common.HexToAddress(address)
	s.ensureStoredABI(addr)
	return contractInfoOf(addr)
}

// LoadContractABI opens a JSON file and stores its ABI for address.
func (s *WalletService) LoadContractABI(address string) (ContractInfo, error) {
	addr := common.HexToAddress(address)
	s.mu.Lock()
	pick := s.pickJSONFile
	url := s.url
	s.mu.Unlock()
	if pick == nil {
		return ContractInfo{}, errNoFilePicker
	}
	path, err := pick()
	if err != nil {
		return ContractInfo{}, err
	}
	if path == "" {
		return contractInfoOf(addr), nil
	}
	return s.loadABIFromPath(addr, path, url)
}

// LoadContractABIFromPath stores the ABI JSON file at path for address.
//
//wails:ignore
func (s *WalletService) LoadContractABIFromPath(address, path string) (ContractInfo, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return ContractInfo{}, errors.New("missing ABI path")
	}
	s.mu.Lock()
	url := s.url
	s.mu.Unlock()
	return s.loadABIFromPath(common.HexToAddress(address), path, url)
}

func (s *WalletService) loadABIFromPath(addr common.Address, path, url string) (ContractInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return ContractInfo{}, err
	}
	defer client.Close()

	c, err := contracts.New(ctx, client, addr)
	if err != nil {
		return ContractInfo{}, err
	}
	if err := c.LoadABI(path); err != nil {
		return ContractInfo{}, err
	}
	s.rememberABIPath(addr, path)
	s.emitABI()
	return contractInfoOf(addr), nil
}

func (s *WalletService) rememberABIPath(addr common.Address, path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	s.mu.Lock()
	if s.abis == nil {
		s.abis = make(map[common.Address]string)
	}
	s.abis[addr] = abs
	s.mu.Unlock()
	s.saveConfig()
}

func (s *WalletService) ensureStoredABI(addr common.Address) {
	s.mu.Lock()
	path := s.abis[addr]
	url := s.url
	s.mu.Unlock()
	if path == "" {
		return
	}
	if c, ok := contracts.Get(addr); ok && c != nil && c.ABIJSON != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return
	}
	defer client.Close()
	s.applyStoredABI(ctx, client, addr)
}

func (s *WalletService) applyStoredABI(ctx context.Context, client *chain.Client, addr common.Address) {
	if client == nil || addr == (common.Address{}) {
		return
	}
	s.mu.Lock()
	path := s.abis[addr]
	s.mu.Unlock()
	if path == "" {
		return
	}
	if c, ok := contracts.Get(addr); ok && c != nil && c.ABIJSON != "" {
		return
	}
	c, err := contracts.New(ctx, client, addr)
	if err != nil {
		return
	}
	if err := c.LoadABI(path); err != nil {
		log.Print(err)
		return
	}
	s.emitABI()
}

// ABISelectors returns selector hex → function name from loaded ABIs.
func (s *WalletService) ABISelectors() map[string]string {
	return contracts.SelectorNames()
}

// ContractCallResult is the Call-tab payload after a view call or send.
type ContractCallResult struct {
	Kind    string  `json:"kind"`
	From    string  `json:"from"`
	Outputs []TxArg `json:"outputs"`
	Hash    string  `json:"hash,omitempty"`
}

// CallContract packs ABI args and eth_calls view/pure methods, or sends a tx.
// Empty from uses the UI-selected wallet. Value is wei hex (empty = 0x0).
func (s *WalletService) CallContract(address, selector string, args []string, from, value string) (ContractCallResult, error) {
	fromAddr, err := s.callFrom(from)
	if err != nil {
		return ContractCallResult{}, err
	}
	wei, err := parseCallValue(value)
	if err != nil {
		return ContractCallResult{}, err
	}
	addr := common.HexToAddress(address)
	c, ok := contracts.Get(addr)
	if !ok {
		return ContractCallResult{}, errors.New("unknown contract")
	}
	calldata, method, err := c.Pack(selector, args)
	if err != nil {
		return ContractCallResult{}, err
	}
	if method.IsConstant() {
		return s.callView(addr, fromAddr, method, calldata)
	}
	if method.StateMutability != "payable" && wei.Sign() != 0 {
		return ContractCallResult{}, errors.New("value not allowed")
	}
	return s.sendContractCall(fromAddr, addr, method, calldata, wei, uiConfirmer{s})
}

// CallContractAuto is CallContract without a confirm dialog.
//
//wails:ignore
func (s *WalletService) CallContractAuto(address, selector string, args []string, from, value string) (ContractCallResult, error) {
	fromAddr, err := s.callFrom(from)
	if err != nil {
		return ContractCallResult{}, err
	}
	wei, err := parseCallValue(value)
	if err != nil {
		return ContractCallResult{}, err
	}
	addr := common.HexToAddress(address)
	c, ok := contracts.Get(addr)
	if !ok {
		return ContractCallResult{}, errors.New("unknown contract")
	}
	calldata, method, err := c.Pack(selector, args)
	if err != nil {
		return ContractCallResult{}, err
	}
	if method.IsConstant() {
		return s.callView(addr, fromAddr, method, calldata)
	}
	if method.StateMutability != "payable" && wei.Sign() != 0 {
		return ContractCallResult{}, errors.New("value not allowed")
	}
	return s.sendContractCall(fromAddr, addr, method, calldata, wei, approveAll{})
}

func (s *WalletService) sendContractCall(fromAddr, addr common.Address, method abi.Method, calldata []byte, wei *big.Int, confirm provider.Confirmer) (ContractCallResult, error) {
	tx := map[string]any{
		"from": fromAddr.Hex(),
		"to":   addr.Hex(),
		"data": hexutil.Encode(calldata),
	}
	if method.StateMutability == "payable" {
		tx["value"] = hexutil.EncodeBig(wei)
	}
	result, err := s.requestWithConfirm(fromAddr, "eth_sendTransaction", []any{tx}, confirm)
	if err != nil {
		return ContractCallResult{}, err
	}
	hash := ""
	switch v := result.(type) {
	case string:
		hash = v
	case nil:
	default:
		hash = fmt.Sprint(v)
	}
	return ContractCallResult{
		Kind:    "send",
		From:    fromAddr.Hex(),
		Outputs: []TxArg{},
		Hash:    hash,
	}, nil
}

func parseCallValue(value string) (*big.Int, error) {
	s := strings.TrimSpace(value)
	if s == "" {
		return big.NewInt(0), nil
	}
	n, err := hexutil.DecodeBig(s)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}
	if n.Sign() < 0 {
		return nil, errors.New("invalid value")
	}
	return n, nil
}

func (s *WalletService) callFrom(from string) (common.Address, error) {
	var addr common.Address
	if strings.TrimSpace(from) == "" {
		s.mu.Lock()
		addr = s.selected
		s.mu.Unlock()
	} else {
		addr = common.HexToAddress(from)
	}
	if _, ok := wallets.Find(addr); !ok {
		return common.Address{}, ErrUnknownAccount
	}
	return addr, nil
}

func (s *WalletService) callView(addr, from common.Address, method abi.Method, calldata []byte) (ContractCallResult, error) {
	s.mu.Lock()
	url := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return ContractCallResult{}, err
	}
	defer client.Close()

	out, err := client.Call(ctx, ethereum.CallMsg{
		From: from,
		To:   &addr,
		Data: calldata,
	})
	if err != nil {
		return ContractCallResult{}, err
	}
	outputs, err := decodeOutputs(method, out)
	if err != nil {
		return ContractCallResult{}, err
	}
	return ContractCallResult{
		Kind:    "view",
		From:    from.Hex(),
		Outputs: outputs,
	}, nil
}

func decodeOutputs(method abi.Method, data []byte) ([]TxArg, error) {
	if len(method.Outputs) == 0 {
		return []TxArg{}, nil
	}
	vals, err := method.Outputs.Unpack(data)
	if err != nil {
		return nil, err
	}
	return argsFrom(method.Outputs, vals), nil
}

func formatValue(v any) string {
	switch x := v.(type) {
	case *big.Int:
		return x.String()
	case common.Address:
		return x.Hex()
	case []byte:
		return hexutil.Encode(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	default:
		rv := reflect.ValueOf(v)
		if rv.IsValid() && rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
			b := make([]byte, rv.Len())
			reflect.Copy(reflect.ValueOf(b), rv)
			return hexutil.Encode(b)
		}
		return fmt.Sprint(v)
	}
}

func (s *WalletService) emitABI() {
	s.mu.Lock()
	emit := s.emit
	s.mu.Unlock()
	if emit != nil {
		emit("abi", s.ABISelectors())
	}
}

func contractInfoOf(addr common.Address) ContractInfo {
	info := ContractInfo{
		Address:   addr.Hex(),
		Events:    []ContractEvent{},
		Functions: []ContractFunction{},
	}
	if c, ok := contracts.Get(addr); ok {
		info.Name = c.Name
		info.ABI = c.ABIJSON
		info.Events, info.Functions = membersFromABI(c.ABI)
		info.EIP712 = eip712Info(c.EIP712)
	}
	return info
}

func eip712Info(src *contracts.EIP712) *EIP712Info {
	if src == nil || (src.PrimaryType == "" && len(src.Types) == 0) {
		return nil
	}
	types := make(map[string][]EIP712Field, len(src.Types))
	for name, fields := range src.Types {
		out := make([]EIP712Field, len(fields))
		for i, f := range fields {
			out[i] = EIP712Field{Name: f.Name, Type: f.Type}
		}
		types[name] = out
	}
	return &EIP712Info{PrimaryType: src.PrimaryType, Types: types}
}

func membersFromABI(spec abi.ABI) ([]ContractEvent, []ContractFunction) {
	events := make([]ContractEvent, 0, len(spec.Events))
	for _, ev := range spec.Events {
		events = append(events, ContractEvent{
			Name:   ev.RawName,
			Inputs: contractArgs(ev.Inputs),
		})
	}
	slices.SortFunc(events, func(a, b ContractEvent) int {
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(argsKey(a.Inputs), argsKey(b.Inputs))
	})

	fns := make([]ContractFunction, 0, len(spec.Methods))
	for _, m := range spec.Methods {
		fns = append(fns, ContractFunction{
			StateMutability: m.StateMutability,
			Name:            m.RawName,
			Selector:        hexutil.Encode(m.ID),
			Inputs:          contractArgs(m.Inputs),
			Outputs:         contractArgs(m.Outputs),
		})
	}
	slices.SortFunc(fns, func(a, b ContractFunction) int {
		if n := strings.Compare(a.Name, b.Name); n != 0 {
			return n
		}
		return strings.Compare(argsKey(a.Inputs), argsKey(b.Inputs))
	})
	return events, fns
}

func contractArgs(args abi.Arguments) []ContractArg {
	out := make([]ContractArg, len(args))
	for i, arg := range args {
		out[i] = ContractArg{
			Type:    arg.Type.String(),
			Name:    arg.Name,
			Indexed: arg.Indexed,
		}
	}
	return out
}

func argsKey(args []ContractArg) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = arg.Type + " " + arg.Name
	}
	return strings.Join(parts, ",")
}
