package walletservice

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http/httptest"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const transferGasUsed = 21000

type ingestEthAPI struct {
	chainID   *big.Int
	balances  map[common.Address]*big.Int
	blockJSON map[string]any
	receipt   map[string]any
}

func (a *ingestEthAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(a.chainID)
}

func (a *ingestEthAPI) BlockNumber() hexutil.Uint64 {
	return 1
}

func (a *ingestEthAPI) GetBalance(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) (*hexutil.Big, error) {
	if a.balances == nil {
		return (*hexutil.Big)(big.NewInt(0)), nil
	}
	if bal, ok := a.balances[address]; ok {
		return (*hexutil.Big)(new(big.Int).Set(bal)), nil
	}
	return (*hexutil.Big)(big.NewInt(0)), nil
}

func (a *ingestEthAPI) GetBlockByNumber(_ context.Context, num rpc.BlockNumber, _ bool) (map[string]any, error) {
	if num != 1 && num != rpc.LatestBlockNumber {
		return nil, nil
	}
	return a.blockJSON, nil
}

func (a *ingestEthAPI) GetTransactionReceipt(_ context.Context, _ common.Hash) (map[string]any, error) {
	return a.receipt, nil
}

type ingestDebugAPI struct {
	diff map[string]any
}

func (a *ingestDebugAPI) TraceTransaction(_ common.Hash, _ map[string]any) (map[string]any, error) {
	return a.diff, nil
}

func twoEther() *big.Int {
	return new(big.Int).Mul(big.NewInt(2), big.NewInt(params.Ether))
}

func signedTwoEthTransfer(t *testing.T) *types.Transaction {
	t.Helper()
	from := wallets.Accounts()[0]
	to := wallets.Accounts()[1].Address
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    0,
		GasPrice: big.NewInt(1),
		Gas:      transferGasUsed,
		To:       &to,
		Value:    twoEther(),
	})
	signed, err := types.SignTx(tx, types.LatestSignerForChainID(big.NewInt(31337)), from.PrivateKey)
	require.NoError(t, err)
	return signed
}

func transferHeader() *types.Header {
	return &types.Header{
		UncleHash:   types.EmptyUncleHash,
		Root:        types.EmptyRootHash,
		TxHash:      types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash,
		Difficulty:  big.NewInt(0),
		Number:      big.NewInt(1),
		GasLimit:    30_000_000,
		GasUsed:     transferGasUsed,
		Time:        1,
	}
}

func rpcBlockJSON(t *testing.T, header *types.Header, signed *types.Transaction, from common.Address) map[string]any {
	t.Helper()
	raw, err := header.MarshalJSON()
	require.NoError(t, err)
	var block map[string]any
	require.NoError(t, json.Unmarshal(raw, &block))
	block["hash"] = header.Hash().Hex()
	block["uncles"] = []any{}

	txRaw, err := signed.MarshalJSON()
	require.NoError(t, err)
	var txm map[string]any
	require.NoError(t, json.Unmarshal(txRaw, &txm))
	txm["from"] = from.Hex()
	txm["blockHash"] = header.Hash().Hex()
	txm["blockNumber"] = hexutil.EncodeUint64(header.Number.Uint64())
	txm["transactionIndex"] = "0x0"
	block["transactions"] = []any{txm}
	return block
}

func startTransferRPC(t *testing.T) (*httptest.Server, *types.Transaction) {
	t.Helper()
	from := wallets.Accounts()[0].Address
	to := wallets.Accounts()[1].Address
	signed := signedTwoEthTransfer(t)
	header := transferHeader()
	header.TxHash = signed.Hash()
	fee := big.NewInt(transferGasUsed) // gasPrice 1
	preFrom := new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether))
	postFrom := new(big.Int).Sub(preFrom, twoEther())
	postFrom.Sub(postFrom, fee)

	balances := make(map[common.Address]*big.Int)
	for _, acc := range wallets.Accounts() {
		balances[acc.Address] = new(big.Int).Mul(big.NewInt(10000), big.NewInt(params.Ether))
	}

	eth := &ingestEthAPI{
		chainID:   big.NewInt(31337),
		balances:  balances,
		blockJSON: rpcBlockJSON(t, header, signed, from),
		receipt: map[string]any{
			"transactionHash":   signed.Hash(),
			"transactionIndex":  hexutil.Uint64(0),
			"blockHash":         header.Hash(),
			"blockNumber":       hexutil.Uint64(1),
			"from":              from,
			"to":                to,
			"gasUsed":           hexutil.Uint64(transferGasUsed),
			"cumulativeGasUsed": hexutil.Uint64(transferGasUsed),
			"logs":              []*types.Log{},
			"logsBloom":         types.Bloom{},
			"status":            hexutil.Uint64(1),
			"effectiveGasPrice": (*hexutil.Big)(big.NewInt(1)),
			"type":              hexutil.Uint64(0),
		},
	}
	debug := &ingestDebugAPI{diff: map[string]any{
		"pre": map[string]any{
			from.Hex(): map[string]any{"balance": (*hexutil.Big)(preFrom)},
			to.Hex():   map[string]any{"balance": (*hexutil.Big)(big.NewInt(0))},
		},
		"post": map[string]any{
			from.Hex(): map[string]any{"balance": (*hexutil.Big)(postFrom)},
			to.Hex():   map[string]any{"balance": (*hexutil.Big)(twoEther())},
		},
	}}

	rpcSrv := rpc.NewServer()
	t.Cleanup(rpcSrv.Stop)
	require.NoError(t, rpcSrv.RegisterName("web3", &watchWeb3API{version: "anvil/v1.0.0"}))
	require.NoError(t, rpcSrv.RegisterName("eth", eth))
	require.NoError(t, rpcSrv.RegisterName("debug", debug))
	httpSrv := httptest.NewServer(rpcSrv)
	t.Cleanup(httpSrv.Close)
	return httpSrv, signed
}

func TestTransactions_catchUpLoadsMinedTransfer(t *testing.T) {
	srv, signed := startTransferRPC(t)
	from := wallets.Accounts()[0].Address.Hex()
	to := wallets.Accounts()[1].Address.Hex()

	svc := NewWithURL(srv.URL)
	got := svc.Transactions()
	require.Len(t, got, 1)
	require.Equal(t, signed.Hash().Hex(), got[0].Hash)
	require.Equal(t, from, got[0].From)
	require.Equal(t, to, got[0].To)
	require.Equal(t, "transfer", got[0].Kind)
	require.Equal(t, "final", got[0].State)
	require.Equal(t, "-"+twoEther().String(), got[0].BalanceChanges[from])
	require.Equal(t, twoEther().String(), got[0].BalanceChanges[to])
	require.NotContains(t, got[0].BalanceChanges[from], "0x")
}

func TestBalanceChanges_decimalNotHex(t *testing.T) {
	from := wallets.Accounts()[0].Address
	to := wallets.Accounts()[1].Address
	fee := big.NewInt(transferGasUsed)
	preFrom := new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether))
	postFrom := new(big.Int).Sub(preFrom, twoEther())
	postFrom.Sub(postFrom, fee)

	diff := &chain.TxDiff{
		Pre: map[common.Address]chain.TraceAccount{
			from: {Balance: (*hexutil.Big)(preFrom)},
			to:   {Balance: (*hexutil.Big)(big.NewInt(0))},
		},
		Post: map[common.Address]chain.TraceAccount{
			from: {Balance: (*hexutil.Big)(postFrom)},
			to:   {Balance: (*hexutil.Big)(twoEther())},
		},
	}
	receipt := &types.Receipt{GasUsed: transferGasUsed, EffectiveGasPrice: big.NewInt(1)}
	got := balanceChanges(diff, from, to.Hex(), receipt)
	require.Equal(t, "-"+twoEther().String(), got[from.Hex()])
	require.Equal(t, twoEther().String(), got[to.Hex()])
	require.NotContains(t, got[from.Hex()], "0x")
	require.NotContains(t, got[to.Hex()], "0x")
}
