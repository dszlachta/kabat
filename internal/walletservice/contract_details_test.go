package walletservice

import (
	"context"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestContractDetails_unknown(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	_, err := svc.ContractDetails("0x1111111111111111111111111111111111111111")
	require.ErrorIs(t, err, ErrUnknownContract)
}

func TestContractDetails_create(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	code := []byte{0x60, 0x00, 0x60, 0x00, 0xfd}
	bal := new(big.Int).SetUint64(1e18)
	srv := detailsTestServer(t, addr, code, bal)
	svc := NewWithURL(srv.URL)
	from := wallets.Accounts()[0].Address.Hex()
	hash := "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	svc.addTx(StoredTx{
		Hash:        hash,
		BlockNumber: 4,
		From:        from,
		Kind:        "create",
		State:       "final",
		Ok:          true,
		Contract:    addr.Hex(),
	})

	got, err := svc.ContractDetails(addr.Hex())
	require.NoError(t, err)
	require.Equal(t, addr.Hex(), got.Address)
	require.Equal(t, from, got.Deployer)
	require.Equal(t, hash, got.Hash)
	require.Equal(t, uint64(4), got.BlockNumber)
	require.Equal(t, hexutil.EncodeBig(bal), got.Balance)
	require.Equal(t, len(code), got.CodeSize)
	require.Equal(t, hexutil.Encode(code), got.Bytecode)
	require.False(t, got.ABILoaded)
	require.Zero(t, got.Functions)
	require.Zero(t, got.Events)
	require.Empty(t, got.Constructor)
	require.Empty(t, got.Name)
}

func TestContractDetails_abiLoaded(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	code := []byte{0xff}
	srv := detailsTestServer(t, addr, code, big.NewInt(0))
	svc := NewWithURL(srv.URL)
	svc.addTx(StoredTx{
		Hash:        "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		BlockNumber: 4,
		From:        wallets.Accounts()[0].Address.Hex(),
		Kind:        "create",
		State:       "final",
		Ok:          true,
		Contract:    addr.Hex(),
	})
	svc.SetPickJSONFile(func() (string, error) {
		return filepath.Join("..", "contracts", "testdata", "abi_members.json"), nil
	})
	_, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	c, ok := contracts.Get(addr)
	require.True(t, ok)
	c.SetName("Token")

	got, err := svc.ContractDetails(addr.Hex())
	require.NoError(t, err)
	require.True(t, got.ABILoaded)
	require.Equal(t, "Token", got.Name)
	require.Equal(t, 2, got.Functions)
	require.Equal(t, 1, got.Events)
	require.Empty(t, got.Constructor)
}

func TestDecodeConstructor_appendedArgs(t *testing.T) {
	spec, err := abi.JSON(strings.NewReader(`[
		{
			"type": "constructor",
			"stateMutability": "nonpayable",
			"inputs": [
				{"name": "owner", "type": "address"},
				{"name": "supply", "type": "uint256"}
			]
		}
	]`))
	require.NoError(t, err)
	runtime := []byte{0x60, 0x00, 0x56}
	owner := common.HexToAddress("0x1111111111111111111111111111111111111111")
	supply := big.NewInt(1000)
	packed, err := spec.Constructor.Inputs.Pack(owner, supply)
	require.NoError(t, err)
	deploy := append(append([]byte{0xfe}, runtime...), packed...)

	args := decodeConstructor(&contracts.Contract{ABI: spec}, runtime, deploy)
	require.Len(t, args, 2)
	require.Equal(t, "address", args[0].Type)
	require.Equal(t, "owner", args[0].Name)
	require.Equal(t, owner.Hex(), args[0].Text)
	require.Equal(t, "uint256", args[1].Type)
	require.Equal(t, "supply", args[1].Name)
	require.Equal(t, "1000", args[1].Text)
}

func TestDecodeConstructor_noInputs(t *testing.T) {
	require.Empty(t, decodeConstructor(&contracts.Contract{}, []byte{0x60}, []byte{0x60, 0x00}))
}

type detailsAPI struct {
	codes    map[common.Address][]byte
	balances map[common.Address]*big.Int
}

func (detailsAPI) ClientVersion() string { return "anvil/v1.0.0" }

func (a *detailsAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(31337))
}

func (a *detailsAPI) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func (a *detailsAPI) GetBalance(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	if a.balances == nil {
		return (*hexutil.Big)(big.NewInt(0)), nil
	}
	if bal, ok := a.balances[address]; ok {
		return (*hexutil.Big)(new(big.Int).Set(bal)), nil
	}
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func detailsTestServer(t *testing.T, addr common.Address, code []byte, bal *big.Int) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	api := &detailsAPI{
		codes:    map[common.Address][]byte{addr: code},
		balances: map[common.Address]*big.Int{addr: bal},
	}
	require.NoError(t, srv.RegisterName("web3", api))
	require.NoError(t, srv.RegisterName("eth", api))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}
