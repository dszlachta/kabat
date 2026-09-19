package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/jsonfmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ErrNotContract is returned when eth_getCode is empty.
var ErrNotContract = errors.New("no bytecode at address")

// Contract is a known on-chain contract (name, deployed code, ABI).
type Contract struct {
	Address  common.Address
	Name     string
	Bytecode []byte
	ABI      abi.ABI
	ABIJSON  string
	EIP712   *EIP712
}

var (
	mu         sync.RWMutex
	byAddr     = map[common.Address]*Contract{}
	bySelector = map[[4]byte]*Contract{}
)

// New loads deployed bytecode for addr and stores it in the process map.
// An existing entry keeps Name and ABI; bytecode is refreshed.
func New(ctx context.Context, client *chain.Client, addr common.Address) (*Contract, error) {
	code, err := client.CodeAt(ctx, addr)
	if err != nil {
		return nil, err
	}
	if len(code) == 0 {
		return nil, ErrNotContract
	}
	mu.Lock()
	defer mu.Unlock()
	if existing, ok := byAddr[addr]; ok {
		existing.Bytecode = append([]byte(nil), code...)
		return existing, nil
	}
	c := &Contract{Address: addr, Bytecode: append([]byte(nil), code...)}
	byAddr[addr] = c
	return c, nil
}

// Get returns the stored contract for addr.
func Get(addr common.Address) (*Contract, bool) {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := byAddr[addr]
	return c, ok
}

// BySelector returns the contract indexed for the 4-byte selector.
func BySelector(selector []byte) (*Contract, bool) {
	id, ok := selectorID(selector)
	if !ok {
		return nil, false
	}
	mu.RLock()
	defer mu.RUnlock()
	c, found := bySelector[id]
	return c, found
}

// MethodName returns the ABI function name for selector, or "" if unknown.
func MethodName(_ common.Address, selector []byte) string {
	return MethodBySelector(selector)
}

// MethodBySelector returns the function name indexed for selector, or "".
func MethodBySelector(selector []byte) string {
	c, ok := BySelector(selector)
	if !ok {
		return ""
	}
	return methodNameOf(c, selector)
}

// SelectorNames is selector hex (0x…) → function name.
func SelectorNames() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string]string, len(bySelector))
	for id, c := range bySelector {
		name := methodNameOf(c, id[:])
		if name == "" {
			continue
		}
		out[hexutil.Encode(id[:])] = name
	}
	return out
}

func methodNameOf(c *Contract, selector []byte) string {
	if c == nil {
		return ""
	}
	m, err := c.ABI.MethodById(selector)
	if err != nil || m == nil {
		return ""
	}
	return m.RawName
}

// SetName sets a custom display name.
func (c *Contract) SetName(name string) {
	if c == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	c.Name = name
}

// LoadABI reads a JSON ABI array or a Foundry/solc artifact with an "abi" field.
func (c *Contract) LoadABI(path string) error {
	if c == nil {
		return ErrNotContract
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	parsed, pretty, typed, err := parseABI(b)
	if err != nil {
		return err
	}
	if typed == nil {
		typed = DeriveEIP712(parsed)
	}
	mu.Lock()
	defer mu.Unlock()
	unindexSelectors(c)
	c.ABI = parsed
	c.ABIJSON = pretty
	c.EIP712 = typed
	indexSelectors(c)
	return nil
}

func selectorID(selector []byte) ([4]byte, bool) {
	var id [4]byte
	if len(selector) < 4 {
		return id, false
	}
	copy(id[:], selector)
	return id, true
}

func unindexSelectors(c *Contract) {
	for _, m := range c.ABI.Methods {
		id, ok := selectorID(m.ID)
		if !ok {
			continue
		}
		if bySelector[id] == c {
			delete(bySelector, id)
		}
	}
}

func indexSelectors(c *Contract) {
	for _, m := range c.ABI.Methods {
		id, ok := selectorID(m.ID)
		if !ok {
			continue
		}
		bySelector[id] = c
	}
}

func parseABI(b []byte) (abi.ABI, string, *EIP712, error) {
	var peek any
	if err := json.Unmarshal(b, &peek); err != nil {
		return abi.ABI{}, "", nil, err
	}
	var raw []byte
	var typed *EIP712
	switch v := peek.(type) {
	case []any:
		raw = b
	case map[string]any:
		field, ok := v["abi"]
		if !ok {
			return abi.ABI{}, "", nil, errors.New("missing abi field")
		}
		encoded, err := json.Marshal(field)
		if err != nil {
			return abi.ABI{}, "", nil, err
		}
		raw = encoded
		typed = parseEIP712Value(v["eip712"])
	default:
		return abi.ABI{}, "", nil, errors.New("invalid abi json")
	}
	parsed, err := abi.JSON(bytes.NewReader(raw))
	if err != nil {
		return abi.ABI{}, "", nil, err
	}
	return parsed, jsonfmt.Bytes(raw), typed, nil
}

func reset() {
	mu.Lock()
	defer mu.Unlock()
	byAddr = map[common.Address]*Contract{}
	bySelector = map[[4]byte]*Contract{}
}

// Reset clears the in-memory contract map.
func Reset() {
	reset()
}
