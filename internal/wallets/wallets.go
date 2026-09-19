// Package wallets holds the predefined Anvil development accounts.
//
// These are Foundry's default accounts derived from the well-known mnemonic
//
//	test test test test test test test test test test test junk
//
// at path m/44'/60'/0'/0/i. The keys are public, static, and must never be
// used with real funds.
package wallets

import (
	"crypto/ecdsa"
	"errors"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Account is a local Anvil development wallet.
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	Address    common.Address
}

var (
	ErrInvalidPrivateKey = errors.New("invalid private key")
	ErrDuplicateWallet   = errors.New("wallet already exists")
)

// Default private keys for Anvil accounts 0–9.
var privateKeys = []string{
	"ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80",
	"59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d",
	"5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a",
	"7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6",
	"47e179ec197488593b187f80a00eb0da91f1b9d0b13f8733639f19c30a34926a",
	"8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba",
	"92db14e403b83dfe3df233f83dfa3a0d7096f21ca9b0d6d6b8d88b2b4ec1564e",
	"4bbbf85ce3377467afe5d46f804f221813b2bb87f24d81f60f1fcdbf7cbf4356",
	"dbda1821b80551c9d65939329250298aa3472ba22feea921c0cf5d620ea67b97",
	"2a871d0798f97d79848a013d4936a73bf4cc922c825d33c1cf7073dff6d409c6",
}

var (
	mu          sync.Mutex
	accounts    []Account
	extra       []Account
	accountsErr error
)

func init() {
	accounts, accountsErr = parseAccounts()
}

func parseAccounts() ([]Account, error) {
	out := make([]Account, len(privateKeys))
	for i, hexKey := range privateKeys {
		key, err := crypto.HexToECDSA(hexKey)
		if err != nil {
			return nil, err
		}
		out[i] = Account{
			PrivateKey: key,
			Address:    crypto.PubkeyToAddress(key.PublicKey),
		}
	}
	return out, nil
}

// LoadError is the result of parsing the predefined Anvil keys at init.
func LoadError() error {
	return accountsErr
}

// Accounts returns the default Anvil wallets plus any added this session.
func Accounts() []Account {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Account, 0, len(accounts)+len(extra))
	out = append(out, accounts...)
	out = append(out, extra...)
	return out
}

// Addresses returns the checksummed hex addresses of the known wallets.
func Addresses() []string {
	accs := Accounts()
	out := make([]string, len(accs))
	for i, acc := range accs {
		out[i] = acc.Address.Hex()
	}
	return out
}

// Find returns the known wallet with the given address.
func Find(addr common.Address) (Account, bool) {
	mu.Lock()
	defer mu.Unlock()
	for _, acc := range accounts {
		if acc.Address == addr {
			return acc, true
		}
	}
	for _, acc := range extra {
		if acc.Address == addr {
			return acc, true
		}
	}
	return Account{}, false
}

// AddRandom generates a new ECDSA key and appends it to the known wallets.
func AddRandom() (Account, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return Account{}, err
	}
	return addKey(key)
}

// AddPrivateKey parses a hex private key (optional 0x) and appends the wallet.
func AddPrivateKey(hexKey string) (Account, error) {
	key, err := parsePrivateKey(hexKey)
	if err != nil {
		return Account{}, err
	}
	return addKey(key)
}

// ResetAdded drops wallets added this session. Tests use this to restore the
// 10 Anvil defaults.
func ResetAdded() {
	mu.Lock()
	defer mu.Unlock()
	extra = nil
}

func parsePrivateKey(hexKey string) (*ecdsa.PrivateKey, error) {
	hexKey = strings.TrimSpace(hexKey)
	hexKey = strings.TrimPrefix(hexKey, "0x")
	hexKey = strings.TrimPrefix(hexKey, "0X")
	if hexKey == "" {
		return nil, ErrInvalidPrivateKey
	}
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, ErrInvalidPrivateKey
	}
	return key, nil
}

func addKey(key *ecdsa.PrivateKey) (Account, error) {
	acc := Account{
		PrivateKey: key,
		Address:    crypto.PubkeyToAddress(key.PublicKey),
	}
	mu.Lock()
	defer mu.Unlock()
	for _, existing := range accounts {
		if existing.Address == acc.Address {
			return Account{}, ErrDuplicateWallet
		}
	}
	for _, existing := range extra {
		if existing.Address == acc.Address {
			return Account{}, ErrDuplicateWallet
		}
	}
	extra = append(extra, acc)
	return acc, nil
}
