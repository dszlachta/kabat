package contracts

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestLoadABI_derivesEIP712FromMethods(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "eip712_abi.json")))

	require.NotNil(t, c.EIP712)
	require.Equal(t, "Withdraw", c.EIP712.PrimaryType)
	require.Equal(t, []EIP712Field{
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	}, c.EIP712.Types["EIP712Domain"])
	require.Equal(t, []EIP712Field{
		{Name: "paymentId", Type: "uint256"},
		{Name: "recipient", Type: "address"},
		{Name: "action", Type: "uint8"},
	}, c.EIP712.Types["Withdraw"])
}

func TestLoadABI_artifactEIP712Field(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "eip712_artifact.json")))

	require.NotNil(t, c.EIP712)
	require.Equal(t, "Mail", c.EIP712.PrimaryType)
	require.Equal(t, []EIP712Field{
		{Name: "from", Type: "address"},
		{Name: "to", Type: "address"},
		{Name: "contents", Type: "string"},
	}, c.EIP712.Types["Mail"])
	_, ok := c.ABI.Methods["symbol"]
	require.True(t, ok)
}

func TestLoadABI_noEIP712(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "abi.json")))
	require.Nil(t, c.EIP712)
}
