package contracts

import (
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func loadMembers(t *testing.T) *Contract {
	t.Helper()
	reset()
	t.Cleanup(reset)
	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(t.Context(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "abi_members.json")))
	return c
}

func TestPack_balanceOf(t *testing.T) {
	c := loadMembers(t)
	account := common.HexToAddress("0x1111111111111111111111111111111111111111")

	got, method, err := c.Pack("0x70a08231", []string{account.Hex()})
	require.NoError(t, err)
	require.Equal(t, "balanceOf", method.RawName)

	packed, err := c.ABI.Methods["balanceOf"].Inputs.Pack(account)
	require.NoError(t, err)
	require.Equal(t, append(c.ABI.Methods["balanceOf"].ID, packed...), got)
}

func TestPack_uintDecimalAndHex(t *testing.T) {
	c := loadMembers(t)
	to := "0x1111111111111111111111111111111111111111"

	dec, _, err := c.Pack("0xa9059cbb", []string{to, "1"})
	require.NoError(t, err)
	hexVal, _, err := c.Pack("0xa9059cbb", []string{to, "0x1"})
	require.NoError(t, err)
	word, _, err := c.Pack("0xa9059cbb", []string{to, "0x0000000000000000000000000000000000000000000000000000000000000001"})
	require.NoError(t, err)
	require.Equal(t, dec, hexVal)
	require.Equal(t, dec, word)
}

func TestPack_invalidNumeric(t *testing.T) {
	c := loadMembers(t)
	to := "0x1111111111111111111111111111111111111111"
	_, _, err := c.Pack("0xa9059cbb", []string{to, "not-a-number"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "amount")

	_, _, err = c.Pack("0xa9059cbb", []string{to, ""})
	require.Error(t, err)
}

func TestPack_wrongArityAndSelector(t *testing.T) {
	c := loadMembers(t)
	_, _, err := c.Pack("0x70a08231", []string{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "expected 1")

	_, _, err = c.Pack("0xdeadbeef", []string{"0x1111111111111111111111111111111111111111"})
	require.Error(t, err)
}
