package wallets_test

import (
	"encoding/hex"
	"testing"

	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

// Canonical Anvil default addresses derived from
// mnemonic: test test test test test test test test test test test junk
// path: m/44'/60'/0'/0/i
var anvilAddresses = []string{
	"0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
	"0x70997970C51812dc3A010C7d01b50e0d17dc79C8",
	"0x3C44CdDdB6a900fa2b585dd299e03d12FA4293BC",
	"0x90F79bf6EB2c4f870365E785982E1f101E93b906",
	"0x15d34AAf54267DB7D7c367839AAf71A00a2C6A65",
	"0x9965507D1a55bcC2695C58ba16FB37d819B0A4dc",
	"0x976EA74026E726554dB657fA54763abd0C3a0aa9",
	"0x14dC79964da2C08b23698B3D3cc7Ca32193d9955",
	"0x23618e81E3f5cdF7f54C3d65f7FBc0aBf5B21E8f",
	"0xa0Ee7A142d267C1f36714E4a8F75612F20a79720",
}

func TestLoadError_nil(t *testing.T) {
	require.NoError(t, wallets.LoadError())
}

func TestAccounts_hasTenAnvilDefaults(t *testing.T) {
	wallets.ResetAdded()
	require.Len(t, wallets.Accounts(), 10)
}

func TestAccounts_addressesMatchAnvilDefaults(t *testing.T) {
	wallets.ResetAdded()
	accounts := wallets.Accounts()
	require.Len(t, accounts, len(anvilAddresses))
	for i, want := range anvilAddresses {
		require.Equal(t, want, accounts[i].Address.Hex())
	}
}

func TestAccountAddresses_returnsChecksummedHex(t *testing.T) {
	wallets.ResetAdded()
	require.Equal(t, anvilAddresses, wallets.Addresses())
}

func TestAddRandom_appendsWallet(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	acc, err := wallets.AddRandom()
	require.NoError(t, err)
	require.NotEqual(t, "0x0000000000000000000000000000000000000000", acc.Address.Hex())
	got, ok := wallets.Find(acc.Address)
	require.True(t, ok)
	require.Equal(t, acc.Address, got.Address)
	require.Len(t, wallets.Accounts(), 11)
}

func TestAddPrivateKey_acceptsPrefixedHex(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	hexKey := hex.EncodeToString(crypto.FromECDSA(key))
	acc, err := wallets.AddPrivateKey("  0x" + hexKey + "\n")
	require.NoError(t, err)
	require.Equal(t, crypto.PubkeyToAddress(key.PublicKey), acc.Address)
	require.Len(t, wallets.Accounts(), 11)
}

func TestAddPrivateKey_invalid(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	for _, hexKey := range []string{"", "  ", "nope", "0xzz", "0x1234"} {
		_, err := wallets.AddPrivateKey(hexKey)
		require.ErrorIs(t, err, wallets.ErrInvalidPrivateKey, hexKey)
	}
	require.Len(t, wallets.Accounts(), 10)
}

func TestAddPrivateKey_duplicateAnvil(t *testing.T) {
	t.Cleanup(wallets.ResetAdded)
	wallets.ResetAdded()
	_, err := wallets.AddPrivateKey("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	require.ErrorIs(t, err, wallets.ErrDuplicateWallet)
	require.Len(t, wallets.Accounts(), 10)
}
