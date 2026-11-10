package walletservice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dszlachta/kabat/internal/provider"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// SendETH transfers value wei (hex) from the selected wallet to to.
func (s *WalletService) SendETH(to, value string) (string, error) {
	s.mu.Lock()
	from := s.selected
	s.mu.Unlock()
	return s.sendETH(from, to, value, uiConfirmer{s})
}

// SendETHAuto sends value wei (hex) from address without a confirm dialog.
//
//wails:ignore
func (s *WalletService) SendETHAuto(from, to, value string) (string, error) {
	addr, err := s.callFrom(from)
	if err != nil {
		return "", err
	}
	return s.sendETH(addr, to, value, approveAll{})
}

func (s *WalletService) sendETH(from common.Address, to, value string, confirm provider.Confirmer) (string, error) {
	to = strings.TrimSpace(to)
	if !common.IsHexAddress(to) {
		return "", errors.New("invalid to")
	}
	if strings.TrimSpace(value) == "" {
		return "", errors.New("invalid value")
	}
	wei, err := parseCallValue(value)
	if err != nil {
		return "", err
	}
	if _, ok := wallets.Find(from); !ok {
		return "", ErrUnknownAccount
	}

	tx := map[string]any{
		"from":  from.Hex(),
		"to":    common.HexToAddress(to).Hex(),
		"value": hexutil.EncodeBig(wei),
	}
	result, err := s.requestWithConfirm(from, "eth_sendTransaction", []any{tx}, confirm)
	if err != nil {
		return "", err
	}
	return resultHash(result), nil
}

func resultHash(result any) string {
	switch v := result.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
