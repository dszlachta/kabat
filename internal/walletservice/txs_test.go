package walletservice

import (
	"context"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestAddTx_newestFirstAndByWallet(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	a := wallets.Accounts()[0].Address.Hex()
	b := wallets.Accounts()[1].Address.Hex()
	outside := "0x000000000000000000000000000000000000dEaD"

	svc.addTx(StoredTx{
		Hash:        "0x1111111111111111111111111111111111111111111111111111111111111111",
		BlockNumber: 1, From: a, To: b, Kind: "transfer", State: "final", Ok: true,
	})
	svc.addTx(StoredTx{
		Hash:        "0x2222222222222222222222222222222222222222222222222222222222222222",
		BlockNumber: 2, From: b, To: outside, Kind: "call", State: "final", Ok: false,
	})
	svc.addTx(StoredTx{
		Hash:        "0x1111111111111111111111111111111111111111111111111111111111111111",
		BlockNumber: 1, From: a, To: b, Kind: "transfer", State: "final", Ok: true,
	})

	all := svc.Transactions()
	require.Len(t, all, 2)
	require.Equal(t, "0x2222222222222222222222222222222222222222222222222222222222222222", all[0].Hash)
	require.Equal(t, "0x1111111111111111111111111111111111111111111111111111111111111111", all[1].Hash)

	forA := svc.TransactionsFor(a)
	require.Len(t, forA, 1)
	require.Equal(t, a, forA[0].From)

	forB := svc.TransactionsFor(b)
	require.Len(t, forB, 2)
	require.Equal(t, "0x2222222222222222222222222222222222222222222222222222222222222222", forB[0].Hash)

	require.Empty(t, svc.TransactionsFor(outside))
}

func TestEthChangeExcludingGas(t *testing.T) {
	ether := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))
	fee := big.NewInt(21000 * 1e9)
	pre := new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether))

	postSent := new(big.Int).Sub(pre, ether)
	postSent.Sub(postSent, fee)
	require.Equal(t, new(big.Int).Neg(ether), ethChangeExcludingGas(pre, postSent, fee, true))

	require.Equal(t, ether, ethChangeExcludingGas(big.NewInt(0), ether, fee, false))

	postFailed := new(big.Int).Sub(pre, fee)
	require.Equal(t, int64(0), ethChangeExcludingGas(pre, postFailed, fee, true).Int64())
}

func TestIngestThrough_skipsWhenCaughtUp(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	svc.lastIngested = 99
	require.NoError(t, svc.ingestThrough(context.Background(), 99))
}

func TestContracts_successfulCreatesOnly(t *testing.T) {
	svc := NewWithURL("http://127.0.0.1:1")
	from := wallets.Accounts()[0].Address.Hex()
	to := wallets.Accounts()[1].Address.Hex()
	deployed := "0x1234567890123456789012345678901234567890"

	svc.addTx(StoredTx{
		Hash:        "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BlockNumber: 1, From: from, To: to, Kind: "transfer", State: "final", Ok: true,
	})
	svc.addTx(StoredTx{
		Hash:        "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		BlockNumber: 2, From: from, Kind: "create", State: "final", Ok: false,
		Contract: "0x9999999999999999999999999999999999999999",
	})
	svc.addTx(StoredTx{
		Hash:        "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		BlockNumber: 3, From: from, Kind: "create", State: "final", Ok: true,
	})
	svc.addTx(StoredTx{
		Hash:        "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		BlockNumber: 4, From: from, Kind: "create", State: "final", Ok: true,
		Contract: deployed,
	})

	got := svc.Contracts()
	require.Len(t, got, 1)
	require.Equal(t, deployed, got[0].Address)
	require.Equal(t, uint64(4), got[0].BlockNumber)
	require.Equal(t, "0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", got[0].TxHash)
}

func TestStoredTxFrom_createSetsContract(t *testing.T) {
	from := wallets.Accounts()[0]
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    0,
		GasPrice: big.NewInt(1),
		Gas:      100000,
		Value:    big.NewInt(0),
		Data:     []byte{0x00},
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(31337)), from.PrivateKey)
	require.NoError(t, err)

	contract := common.HexToAddress("0x1111111111111111111111111111111111111111")
	stored, err := storedTxFrom(signed, &types.Receipt{
		Status:          types.ReceiptStatusSuccessful,
		GasUsed:         50000,
		ContractAddress: contract,
	}, 7)
	require.NoError(t, err)
	require.Equal(t, "create", stored.Kind)
	require.True(t, stored.Ok)
	require.Equal(t, contract.Hex(), stored.Contract)

	zero, err := storedTxFrom(signed, &types.Receipt{
		Status: types.ReceiptStatusSuccessful,
	}, 7)
	require.NoError(t, err)
	require.Empty(t, zero.Contract)
}

func TestWithCallNames_usesLoadedABI(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	to := addr.Hex()
	txs := withCallNames([]StoredTx{{
		Kind: "call", To: to, Selector: "0x70a08231",
	}})
	require.Empty(t, txs[0].Method)

	httpSrv := contractsTestServer(t, addr)
	client, err := chain.ConnectURL(context.Background(), httpSrv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	c, err := contracts.New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("..", "contracts", "testdata", "abi_members.json")))

	named := withCallNames([]StoredTx{
		{Kind: "call", To: to, Selector: "0x70a08231"},
		{Kind: "call", To: to, Selector: "0xdeadbeef"},
		{Kind: "transfer", To: to, Selector: "0x70a08231"},
	})
	require.Equal(t, "balanceOf", named[0].Method)
	require.Empty(t, named[1].Method)
	require.Empty(t, named[2].Method)

	svc := NewWithURL("http://127.0.0.1:1")
	svc.addTx(StoredTx{
		Hash: "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		From: wallets.Accounts()[0].Address.Hex(),
		To:   to, Kind: "call", Selector: "0xa9059cbb", State: "final",
	})
	got := svc.Transactions()
	require.Equal(t, "transfer", got[0].Method)
}

func TestLoadContractABI_emitsNamesForExistingCalls(t *testing.T) {
	contracts.Reset()
	t.Cleanup(contracts.Reset)
	addr := common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddddd")
	httpSrv := contractsTestServer(t, addr)
	svc := NewWithURL(httpSrv.URL)
	svc.addTx(StoredTx{
		Hash:     "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		From:     wallets.Accounts()[0].Address.Hex(),
		To:       addr.Hex(),
		Kind:     "call",
		Selector: "0x70a08231",
		State:    "final",
	})
	require.Empty(t, svc.Transactions()[0].Method)

	var emitted map[string]string
	svc.SetEmitter(func(event string, data any) {
		if event == "abi" {
			emitted = data.(map[string]string)
		}
	})
	svc.SetPickJSONFile(func() (string, error) {
		return filepath.Join("..", "contracts", "testdata", "abi_members.json"), nil
	})

	_, err := svc.LoadContractABI(addr.Hex())
	require.NoError(t, err)
	require.Equal(t, "balanceOf", svc.ABISelectors()["0x70a08231"])
	require.Equal(t, "balanceOf", emitted["0x70a08231"])
	require.Equal(t, "transfer", emitted["0xa9059cbb"])
}

type codeAPI struct {
	codes map[common.Address][]byte
}

func (codeAPI) ClientVersion() string { return "anvil/v1.0.0" }

func (a *codeAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(31337))
}

func (a *codeAPI) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func contractsTestServer(t *testing.T, addr common.Address) *httptest.Server {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	api := &codeAPI{codes: map[common.Address][]byte{addr: {0xff}}}
	require.NoError(t, srv.RegisterName("web3", api))
	require.NoError(t, srv.RegisterName("eth", api))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	return httpSrv
}
