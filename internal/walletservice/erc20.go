package walletservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
	"unicode"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const erc20ABIJSON = `[
  {"name":"name","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"string"}]},
  {"name":"symbol","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"string"}]},
  {"name":"decimals","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint8"}]},
  {"name":"totalSupply","type":"function","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
  {"name":"balanceOf","type":"function","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}]},
  {"name":"transfer","type":"function","stateMutability":"nonpayable","inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],"outputs":[{"type":"bool"}]}
]`

var (
	erc20ABI        abi.ABI
	selTransfer     = []byte{0xa9, 0x05, 0x9c, 0xbb}
	selBalanceOf    = []byte{0x70, 0xa0, 0x82, 0x31}
	errInvalidToken = errors.New("invalid token")
	errNotERC20     = errors.New("not an ERC-20")
	errInvalidAmt   = errors.New("invalid amount")
)

func init() {
	parsed, err := abi.JSON(strings.NewReader(erc20ABIJSON))
	if err != nil {
		panic(err)
	}
	erc20ABI = parsed
}

// ERC20Token is one probed token for the Send dialog picker.
type ERC20Token struct {
	Address  string `json:"address"`
	Name     string `json:"name,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Decimals uint8  `json:"decimals"`
	Balance  string `json:"balance,omitempty"`
}

// ERC20Summary is the ERC-20 block on contract details.
type ERC20Summary struct {
	Name        string `json:"name,omitempty"`
	Symbol      string `json:"symbol,omitempty"`
	Decimals    uint8  `json:"decimals"`
	TotalSupply string `json:"totalSupply,omitempty"`
}

// Tokens returns ERC-20s among successful creates, newest first.
func (s *WalletService) Tokens() ([]ERC20Token, error) {
	creates := s.Contracts()
	s.mu.Lock()
	url := s.url
	selected := s.selected
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	out := make([]ERC20Token, 0)
	for _, c := range creates {
		addr := common.HexToAddress(c.Address)
		code, err := client.CodeAt(ctx, addr)
		if err != nil {
			continue
		}
		sum := probeERC20(ctx, client, addr, code)
		if sum == nil {
			continue
		}
		tok := ERC20Token{
			Address:  addr.Hex(),
			Name:     sum.Name,
			Symbol:   sum.Symbol,
			Decimals: sum.Decimals,
		}
		if n, err := callBalanceOf(ctx, client, addr, selected); err == nil {
			tok.Balance = formatUnits(n, int(sum.Decimals))
		}
		out = append(out, tok)
	}
	return out, nil
}

// SendERC20 transfers amount token units from the selected wallet.
func (s *WalletService) SendERC20(token, to, amount string) (string, error) {
	s.mu.Lock()
	from := s.selected
	s.mu.Unlock()
	return s.sendERC20(from, token, to, amount, uiConfirmer{s})
}

// SendERC20Auto sends amount token units from address without a confirm dialog.
//
//wails:ignore
func (s *WalletService) SendERC20Auto(from, token, to, amount string) (string, error) {
	addr, err := s.callFrom(from)
	if err != nil {
		return "", err
	}
	return s.sendERC20(addr, token, to, amount, approveAll{})
}

func (s *WalletService) sendERC20(from common.Address, token, to, amount string, confirm provider.Confirmer) (string, error) {
	token = strings.TrimSpace(token)
	if !common.IsHexAddress(token) {
		return "", errInvalidToken
	}
	to = strings.TrimSpace(to)
	if !common.IsHexAddress(to) {
		return "", errors.New("invalid to")
	}
	if _, ok := wallets.Find(from); !ok {
		return "", ErrUnknownAccount
	}

	s.mu.Lock()
	url := s.url
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return "", err
	}
	tokenAddr := common.HexToAddress(token)
	code, err := client.CodeAt(ctx, tokenAddr)
	if err != nil {
		client.Close()
		return "", err
	}
	sum := probeERC20(ctx, client, tokenAddr, code)
	if sum == nil {
		client.Close()
		return "", errNotERC20
	}
	units, err := parseUnits(amount, int(sum.Decimals))
	if err != nil {
		client.Close()
		return "", err
	}
	if bal, err := callBalanceOf(ctx, client, tokenAddr, from); err == nil && bal.Cmp(units) < 0 {
		client.Close()
		sym := sum.Symbol
		if sym == "" {
			sym = "token"
		}
		return "", fmt.Errorf("insufficient %s balance", sym)
	}
	client.Close()
	data, err := erc20ABI.Pack("transfer", common.HexToAddress(to), units)
	if err != nil {
		return "", err
	}
	tx := map[string]any{
		"from": from.Hex(),
		"to":   tokenAddr.Hex(),
		"data": hexutil.Encode(data),
	}
	result, err := s.requestWithConfirm(from, "eth_sendTransaction", []any{tx}, confirm)
	if err != nil {
		return "", err
	}
	return resultHash(result), nil
}

func probeERC20(ctx context.Context, client *chain.Client, addr common.Address, code []byte) *ERC20Summary {
	if !bytes.Contains(code, selTransfer) || !bytes.Contains(code, selBalanceOf) {
		return nil
	}
	decimals, err := callDecimals(ctx, client, addr)
	if err != nil {
		return nil
	}
	out := &ERC20Summary{Decimals: decimals}
	if name, err := callString(ctx, client, addr, "name"); err == nil {
		out.Name = name
	}
	if symbol, err := callString(ctx, client, addr, "symbol"); err == nil {
		out.Symbol = symbol
	}
	if supply, err := callUint256(ctx, client, addr, "totalSupply"); err == nil {
		out.TotalSupply = formatUnits(supply, int(decimals))
	}
	return out
}

func callDecimals(ctx context.Context, client *chain.Client, addr common.Address) (uint8, error) {
	vals, err := erc20Call(ctx, client, addr, "decimals")
	if err != nil || len(vals) == 0 {
		return 0, errNotERC20
	}
	switch v := vals[0].(type) {
	case uint8:
		return v, nil
	case *big.Int:
		if v == nil || !v.IsUint64() || v.Uint64() > 255 {
			return 0, errNotERC20
		}
		return uint8(v.Uint64()), nil
	default:
		return 0, errNotERC20
	}
}

func callString(ctx context.Context, client *chain.Client, addr common.Address, method string) (string, error) {
	vals, err := erc20Call(ctx, client, addr, method)
	if err != nil || len(vals) == 0 {
		return "", err
	}
	s, ok := vals[0].(string)
	if !ok || s == "" {
		return "", errNotERC20
	}
	return s, nil
}

func callUint256(ctx context.Context, client *chain.Client, addr common.Address, method string) (*big.Int, error) {
	return unpackUint256(erc20Call(ctx, client, addr, method))
}

func callBalanceOf(ctx context.Context, client *chain.Client, token, account common.Address) (*big.Int, error) {
	return unpackUint256(erc20Call(ctx, client, token, "balanceOf", account))
}

func unpackUint256(vals []any, err error) (*big.Int, error) {
	if err != nil || len(vals) == 0 {
		return nil, err
	}
	n, ok := vals[0].(*big.Int)
	if !ok || n == nil {
		return nil, errNotERC20
	}
	return n, nil
}

func erc20Call(ctx context.Context, client *chain.Client, addr common.Address, method string, args ...any) ([]any, error) {
	data, err := erc20ABI.Pack(method, args...)
	if err != nil {
		return nil, err
	}
	out, err := client.Call(ctx, ethereum.CallMsg{To: &addr, Data: data})
	if err != nil {
		return nil, err
	}
	return erc20ABI.Unpack(method, out)
}

func parseUnits(s string, decimals int) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "-") || s == "." || strings.Count(s, ".") > 1 {
		return nil, errInvalidAmt
	}
	wholeRaw, fracRaw, _ := strings.Cut(s, ".")
	if wholeRaw != "" && !isDigits(wholeRaw) {
		return nil, errInvalidAmt
	}
	if fracRaw != "" && !isDigits(fracRaw) {
		return nil, errInvalidAmt
	}
	if decimals < 0 || len(fracRaw) > decimals {
		return nil, errInvalidAmt
	}
	whole := big.NewInt(0)
	if wholeRaw != "" {
		if _, ok := whole.SetString(wholeRaw, 10); !ok {
			return nil, errInvalidAmt
		}
	}
	frac := big.NewInt(0)
	if fracRaw != "" {
		padded := fracRaw + strings.Repeat("0", decimals-len(fracRaw))
		if _, ok := frac.SetString(padded, 10); !ok {
			return nil, errInvalidAmt
		}
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	return new(big.Int).Add(new(big.Int).Mul(whole, base), frac), nil
}

func formatUnits(n *big.Int, decimals int) string {
	if n == nil {
		n = big.NewInt(0)
	}
	neg := n.Sign() < 0
	if neg {
		n = new(big.Int).Neg(n)
	}
	if decimals <= 0 {
		if neg {
			return "-" + n.String()
		}
		return n.String()
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(n, base)
	frac := new(big.Int).Mod(n, base)
	out := whole.String()
	if frac.Sign() != 0 {
		padded := frac.String()
		if len(padded) < decimals {
			padded = strings.Repeat("0", decimals-len(padded)) + padded
		}
		out += "." + strings.TrimRight(padded, "0")
	}
	if neg {
		return "-" + out
	}
	return out
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
