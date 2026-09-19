package walletservice

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestTxDetails_unknown(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	_, err := svc.TxDetails("0x1111111111111111111111111111111111111111111111111111111111111111")
	require.ErrorIs(t, err, ErrUnknownTransaction)
}

func TestTxDetails_transfer(t *testing.T) {
	srv, signed := startTransferRPC(t)
	svc := NewWithURL(srv.URL)
	require.NotEmpty(t, svc.Transactions())

	got, err := svc.TxDetails(signed.Hash().Hex())
	require.NoError(t, err)
	require.Equal(t, "transfer", got.Kind)
	require.True(t, got.Ok)
	require.Equal(t, wallets.Accounts()[0].Address.Hex(), got.From)
	require.Equal(t, wallets.Accounts()[1].Address.Hex(), got.To)
	require.Equal(t, signed.Nonce(), got.Nonce)
	require.Equal(t, uint64(transferGasUsed), got.GasUsed)
	require.Equal(t, uint64(transferGasUsed), got.GasLimit)
	require.Equal(t, "0x", got.Calldata)
	require.Empty(t, got.Args)
	require.Empty(t, got.Revert)
	require.NotEmpty(t, got.BalanceChanges)
}

func TestDecodeCalldata_transfer(t *testing.T) {
	c := loadMembersContract(t)
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	amount := big.NewInt(1)
	method := c.ABI.Methods["transfer"]
	packed, err := method.Inputs.Pack(to, amount)
	require.NoError(t, err)
	data := append(append([]byte(nil), method.ID...), packed...)

	args := decodeCalldata(data)
	require.Len(t, args, 2)
	require.Equal(t, "address", args[0].Type)
	require.Equal(t, "to", args[0].Name)
	require.Equal(t, to.Hex(), args[0].Text)
	require.Equal(t, "uint256", args[1].Type)
	require.Equal(t, "amount", args[1].Name)
	require.Equal(t, "1", args[1].Text)
}

func TestDecodeLog_transferEvent(t *testing.T) {
	c := loadMembersContract(t)
	from := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	ev := c.ABI.Events["Transfer"]
	data, err := ev.Inputs.NonIndexed().Pack(big.NewInt(3))
	require.NoError(t, err)
	lg := &types.Log{
		Address: c.Address,
		Topics: []common.Hash{
			ev.ID,
			common.BytesToHash(from.Bytes()),
			common.BytesToHash(to.Bytes()),
		},
		Data: data,
	}
	got := decodeLog(lg)
	require.Equal(t, "Transfer", got.Name)
	require.Equal(t, c.Address.Hex(), got.Address)
	require.Len(t, got.Args, 3)
	require.Equal(t, from.Hex(), got.Args[0].Text)
	require.Equal(t, to.Hex(), got.Args[1].Text)
	require.Equal(t, "3", got.Args[2].Text)
}

func TestRevertFromTrace(t *testing.T) {
	require.Equal(t, "use create_payment", revertFromTrace(&chain.CallTrace{
		RevertReason: "use create_payment",
	}))
	require.Equal(t, "execution reverted", revertFromTrace(&chain.CallTrace{
		Error: "execution reverted",
	}))

	typ, err := abi.NewType("string", "", nil)
	require.NoError(t, err)
	packed, err := abi.Arguments{{Type: typ}}.Pack("nope")
	require.NoError(t, err)
	out := append([]byte{0x08, 0xc3, 0x79, 0xa0}, packed...)
	require.Equal(t, "nope", revertFromTrace(&chain.CallTrace{Output: hexutil.Bytes(out)}))
}

func loadMembersContract(t *testing.T) *contracts.Contract {
	t.Helper()
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	httpSrv := contractsTestServer(t, addr)
	client, err := chain.ConnectURL(t.Context(), httpSrv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	c, err := contracts.New(t.Context(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("..", "contracts", "testdata", "abi_members.json")))
	return c
}
