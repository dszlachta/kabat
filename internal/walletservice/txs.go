package walletservice

import (
	"context"
	"errors"
	"log"
	"math/big"
	"slices"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const ingestRetries = 3
const ingestRetryWait = 50 * time.Millisecond

var errBlockNil = errors.New("block not found")

// StoredTx is a mined transaction kept for the Txs lists.
type StoredTx struct {
	Hash           string            `json:"hash"`
	BlockNumber    uint64            `json:"blockNumber"`
	From           string            `json:"from"`
	To             string            `json:"to"`
	Value          string            `json:"value"`
	Gas            uint64            `json:"gas"`
	Kind           string            `json:"kind"`
	Selector       string            `json:"selector,omitempty"`
	Method         string            `json:"method,omitempty"`
	Ok             bool              `json:"ok"`
	State          string            `json:"state"`
	Contract       string            `json:"contract,omitempty"`
	BalanceChanges map[string]string `json:"balanceChanges,omitempty"`
}

// StoredContract is a successful create from an Anvil wallet.
type StoredContract struct {
	Address     string `json:"address"`
	BlockNumber uint64 `json:"blockNumber"`
	TxHash      string `json:"hash"`
}

func (s *WalletService) addTx(tx StoredTx) {
	hash := common.HexToHash(tx.Hash)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.txHashes[hash]; ok {
		return
	}
	s.txHashes[hash] = struct{}{}
	s.txs = append([]StoredTx{tx}, s.txs...)
	seen := map[common.Address]struct{}{}
	for _, hexAddr := range []string{tx.From, tx.To} {
		if hexAddr == "" {
			continue
		}
		addr := common.HexToAddress(hexAddr)
		if _, ok := wallets.Find(addr); !ok {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		s.txsByWallet[addr] = append([]StoredTx{tx}, s.txsByWallet[addr]...)
	}
}

func (s *WalletService) catchUp(ctx context.Context) error {
	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return err
	}
	latest, err := client.BlockNumber(ctx)
	client.Close()
	if err != nil {
		return err
	}
	return s.ingestThrough(ctx, latest)
}

// Transactions returns mined txs, newest first.
func (s *WalletService) Transactions() []StoredTx {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.catchUp(ctx); err != nil {
		log.Print(err)
	}
	s.mu.Lock()
	out := s.txs
	if out == nil {
		s.mu.Unlock()
		return []StoredTx{}
	}
	out = slices.Clone(out)
	s.mu.Unlock()
	return withCallNames(out)
}

// TransactionsFor returns mined txs involving address (from or to), newest first.
func (s *WalletService) TransactionsFor(address string) []StoredTx {
	addr := common.HexToAddress(address)
	s.mu.Lock()
	got := s.txsByWallet[addr]
	if got == nil {
		s.mu.Unlock()
		return []StoredTx{}
	}
	got = slices.Clone(got)
	s.mu.Unlock()
	return withCallNames(got)
}

// Contracts returns successful wallet creates, newest first.
func (s *WalletService) Contracts() []StoredContract {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.catchUp(ctx); err != nil {
		log.Print(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoredContract, 0)
	for _, tx := range s.txs {
		if tx.Kind != "create" || !tx.Ok || tx.Contract == "" {
			continue
		}
		out = append(out, StoredContract{
			Address:     tx.Contract,
			BlockNumber: tx.BlockNumber,
			TxHash:      tx.Hash,
		})
	}
	return out
}

func (s *WalletService) emitTransactions() {
	s.mu.Lock()
	emit := s.emit
	out := slices.Clone(s.txs)
	if out == nil {
		out = []StoredTx{}
	}
	s.mu.Unlock()
	if emit != nil {
		emit("transactions", withCallNames(out))
	}
}

func withCallNames(txs []StoredTx) []StoredTx {
	for i := range txs {
		txs[i].Method = callMethodName(txs[i])
	}
	return txs
}

func callMethodName(tx StoredTx) string {
	if tx.Kind != "call" || tx.To == "" || tx.Selector == "" {
		return ""
	}
	id, err := hexutil.Decode(tx.Selector)
	if err != nil {
		return ""
	}
	return contracts.MethodName(common.HexToAddress(tx.To), id)
}

func (s *WalletService) ingestThrough(ctx context.Context, to uint64) error {
	s.mu.Lock()
	from := s.lastIngested + 1
	s.mu.Unlock()
	if from > to {
		return nil
	}

	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return err
	}
	defer client.Close()

	for n := from; n <= to; n++ {
		block, err := fetchBlockByNumber(ctx, client, n)
		if err != nil {
			return err
		}
		if err := s.ingestFetchedBlock(ctx, client, block); err != nil {
			return err
		}
		s.mu.Lock()
		s.lastIngested = n
		s.mu.Unlock()
	}
	return nil
}

func fetchBlockByNumber(ctx context.Context, client *chain.Client, n uint64) (*types.Block, error) {
	var last error
	for attempt := 0; attempt < ingestRetries; attempt++ {
		block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(n))
		if err == nil && block != nil {
			return block, nil
		}
		last = err
		if err == nil {
			last = errBlockNil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(ingestRetryWait):
		}
	}
	return nil, last
}

func (s *WalletService) ingestFetchedBlock(ctx context.Context, client *chain.Client, block *types.Block) error {
	if block == nil {
		return nil
	}
	for _, tx := range block.Transactions() {
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			log.Print(err)
			continue
		}
		stored, err := storedTxFrom(tx, receipt, block.NumberU64())
		if err != nil {
			log.Print(err)
			continue
		}
		diff, err := client.TraceTxDiff(ctx, tx.Hash())
		if err != nil {
			log.Print(err)
		} else {
			stored.BalanceChanges = balanceChanges(diff, common.HexToAddress(stored.From), stored.To, receipt)
		}
		s.addTx(stored)
		if !stored.Ok {
			continue
		}
		if stored.Contract != "" {
			s.applyStoredABI(ctx, client, common.HexToAddress(stored.Contract))
		} else if stored.To != "" {
			s.applyStoredABI(ctx, client, common.HexToAddress(stored.To))
		}
	}
	return nil
}

func storedTxFrom(tx *types.Transaction, receipt *types.Receipt, blockNumber uint64) (StoredTx, error) {
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err != nil {
		return StoredTx{}, err
	}

	to := ""
	if tx.To() != nil {
		to = tx.To().Hex()
	}

	kind := "transfer"
	selector := ""
	data := tx.Data()
	if tx.To() == nil {
		kind = "create"
	} else if len(data) > 0 {
		kind = "call"
		n := 4
		if len(data) < n {
			n = len(data)
		}
		selector = hexutil.Encode(data[:n])
	}

	value := "0x0"
	if tx.Value() != nil {
		value = hexutil.EncodeBig(tx.Value())
	}

	gas := uint64(0)
	ok := false
	contract := ""
	if receipt != nil {
		gas = receipt.GasUsed
		ok = receipt.Status == types.ReceiptStatusSuccessful
		if receipt.ContractAddress != (common.Address{}) {
			contract = receipt.ContractAddress.Hex()
		}
	}

	return StoredTx{
		Hash:        tx.Hash().Hex(),
		BlockNumber: blockNumber,
		From:        from.Hex(),
		To:          to,
		Value:       value,
		Gas:         gas,
		Kind:        kind,
		Selector:    selector,
		Ok:          ok,
		Contract:    contract,
		State:       "final",
	}, nil
}

func gasFee(receipt *types.Receipt) *big.Int {
	if receipt == nil || receipt.EffectiveGasPrice == nil {
		return new(big.Int)
	}
	return new(big.Int).Mul(receipt.EffectiveGasPrice, new(big.Int).SetUint64(receipt.GasUsed))
}

func ethChangeExcludingGas(pre, post, fee *big.Int, sender bool) *big.Int {
	if pre == nil {
		pre = new(big.Int)
	}
	if post == nil {
		post = new(big.Int)
	}
	d := new(big.Int).Sub(post, pre)
	if sender && fee != nil {
		d.Add(d, fee)
	}
	return d
}

func balanceChanges(diff *chain.TxDiff, from common.Address, toHex string, receipt *types.Receipt) map[string]string {
	fee := gasFee(receipt)
	out := map[string]string{}
	addrs := []common.Address{from}
	if toHex != "" {
		addrs = append(addrs, common.HexToAddress(toHex))
	}
	seen := map[common.Address]struct{}{}
	for _, addr := range addrs {
		if _, ok := wallets.Find(addr); !ok {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		pre, post := diff.Balance(addr)
		delta := ethChangeExcludingGas(pre, post, fee, addr == from)
		if delta.Sign() == 0 {
			continue
		}
		out[addr.Hex()] = delta.String()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
