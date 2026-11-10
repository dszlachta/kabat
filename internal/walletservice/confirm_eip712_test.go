package walletservice

import (
	"bytes"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/stretchr/testify/require"
)

func TestEIP712SchemaFields_orderAndValues(t *testing.T) {
	schema := &contracts.EIP712{
		PrimaryType: "Withdraw",
		Types: map[string][]contracts.EIP712Field{
			"Withdraw": {
				{Name: "paymentId", Type: "uint256"},
				{Name: "recipient", Type: "address"},
			},
			"EIP712Domain": {
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
			},
		},
	}
	fields := eip712SchemaFields(schema,
		map[string]string{"name": "Escrow", "version": "1"},
		map[string]string{"paymentId": "10"},
	)
	require.Len(t, fields, 2)
	require.Equal(t, "EIP712Domain", fields[0].Name)
	require.Equal(t, "Escrow", fields[0].Fields[0].Text)
	require.Equal(t, "1", fields[0].Fields[1].Text)
	require.Equal(t, "Withdraw", fields[1].Name)
	require.Equal(t, "10", fields[1].Fields[0].Text)
	require.Equal(t, "uint256", fields[1].Fields[0].Type)
	require.Empty(t, fields[1].Fields[1].Text)
}

func TestEIP712SchemaFields_nil(t *testing.T) {
	require.Nil(t, eip712SchemaFields(nil, nil, nil))
}

func TestEIP712SchemaFields_mapsSnakeCaseCallArgs(t *testing.T) {
	schema := &contracts.EIP712{
		PrimaryType: "Withdraw",
		Types: map[string][]contracts.EIP712Field{
			"Withdraw": {
				{Name: "paymentId", Type: "uint256"},
				{Name: "recipient", Type: "address"},
			},
		},
	}
	fields := eip712SchemaFields(schema, nil, map[string]string{"payment_id": "10"})
	require.Equal(t, "10", fields[0].Fields[0].Text)
}

func TestCallArgValues_withdrawPaymentID(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "contracts", "testdata", "eip712_abi.json"))
	require.NoError(t, err)
	spec, err := abi.JSON(bytes.NewReader(raw))
	require.NoError(t, err)
	c := &contracts.Contract{ABI: spec}
	method := spec.Methods["withdraw"]
	packed, err := method.Inputs.Pack(big.NewInt(10), []byte{0xaa})
	require.NoError(t, err)
	got := callArgValues(c, append(method.ID, packed...))
	require.Equal(t, "10", got["payment_id"])
}
