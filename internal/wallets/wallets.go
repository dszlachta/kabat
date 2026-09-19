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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// Account is a local Anvil development wallet.
type Account struct {
	PrivateKey *ecdsa.PrivateKey
	Address    common.Address
}

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
	accounts    []Account
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

// Accounts returns the 10 default Anvil development wallets.
func Accounts() []Account {
	return accounts
}

// Addresses returns the checksummed hex addresses of the default wallets.
func Addresses() []string {
	out := make([]string, len(accounts))
	for i, acc := range accounts {
		out[i] = acc.Address.Hex()
	}
	return out
}

// Find returns the predefined wallet with the given address.
func Find(addr common.Address) (Account, bool) {
	for _, acc := range accounts {
		if acc.Address == addr {
			return acc, true
		}
	}
	return Account{}, false
}
