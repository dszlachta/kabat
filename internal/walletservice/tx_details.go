package walletservice

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

// ErrUnknownTransaction is returned when hash is not in the mined list.
var ErrUnknownTransaction = errors.New("unknown transaction")

// TxArg is one decoded ABI argument or event field.
type TxArg struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Text string `json:"text"`
}

// TxLog is one receipt log, with ABI-decoded fields when known.
type TxLog struct {
	Name    string   `json:"name,omitempty"`
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
	Args    []TxArg  `json:"args"`
}

// TxBalanceChange is one Anvil-wallet ETH delta (gas excluded).
type TxBalanceChange struct {
	Address string `json:"address"`
	Delta   string `json:"delta"`
}

// TxDetails is the transaction-details pane payload.
type TxDetails struct {
	Hash           string            `json:"hash"`
	BlockNumber    uint64            `json:"blockNumber"`
	Ok             bool              `json:"ok"`
	Kind           string            `json:"kind"`
	Method         string            `json:"method,omitempty"`
	Selector       string            `json:"selector,omitempty"`
	From           string            `json:"from"`
	To             string            `json:"to"`
	Contract       string            `json:"contract,omitempty"`
	Value          string            `json:"value"`
	Nonce          uint64            `json:"nonce"`
	GasUsed        uint64            `json:"gasUsed"`
	GasLimit       uint64            `json:"gasLimit"`
	GasPrice       string            `json:"gasPrice"`
	Fee            string            `json:"fee"`
	BalanceChanges []TxBalanceChange `json:"balanceChanges"`
	Calldata       string            `json:"calldata"`
	Args           []TxArg           `json:"args"`
	Revert         string            `json:"revert,omitempty"`
	Logs           []TxLog           `json:"logs"`
}

// TxDetails returns decoded details for a mined transaction.
func (s *WalletService) TxDetails(hash string) (TxDetails, error) {
	h := common.HexToHash(hash)
	stored, ok := s.storedTx(h)
	if !ok {
		return TxDetails{}, ErrUnknownTransaction
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, s.url)
	if err != nil {
		return TxDetails{}, err
	}
	defer client.Close()

	receipt, err := client.TransactionReceipt(ctx, h)
	if err != nil {
		return TxDetails{}, err
	}
	tx, err := txInBlock(ctx, client, stored.BlockNumber, h)
	if err != nil {
		return TxDetails{}, err
	}

	data := tx.Data()
	calldata := hexutil.Encode(data)
	selector := stored.Selector
	if selector == "" && len(data) >= 4 {
		selector = hexutil.Encode(data[:4])
	}

	gasPrice := "0x0"
	if receipt.EffectiveGasPrice != nil {
		gasPrice = hexutil.EncodeBig(receipt.EffectiveGasPrice)
	}
	fee := hexutil.EncodeBig(gasFee(receipt))
	value := stored.Value
	if tx.Value() != nil {
		value = hexutil.EncodeBig(tx.Value())
	}

	okStatus := receipt.Status == types.ReceiptStatusSuccessful
	revert := ""
	if !okStatus {
		revert = revertReason(ctx, client, h)
	}

	logs := make([]TxLog, 0, len(receipt.Logs))
	for _, lg := range receipt.Logs {
		if lg == nil {
			continue
		}
		logs = append(logs, decodeLog(lg))
	}

	out := TxDetails{
		Hash:           stored.Hash,
		BlockNumber:    stored.BlockNumber,
		Ok:             okStatus,
		Kind:           stored.Kind,
		Method:         callMethodName(stored),
		Selector:       selector,
		From:           stored.From,
		To:             stored.To,
		Contract:       stored.Contract,
		Value:          value,
		Nonce:          tx.Nonce(),
		GasUsed:        receipt.GasUsed,
		GasLimit:       tx.Gas(),
		GasPrice:       gasPrice,
		Fee:            fee,
		BalanceChanges: balanceChangeList(stored),
		Calldata:       calldata,
		Args:           decodeCalldata(data),
		Revert:         revert,
		Logs:           logs,
	}
	if out.Args == nil {
		out.Args = []TxArg{}
	}
	if out.Logs == nil {
		out.Logs = []TxLog{}
	}
	if out.BalanceChanges == nil {
		out.BalanceChanges = []TxBalanceChange{}
	}
	return out, nil
}

func (s *WalletService) storedTx(hash common.Hash) (StoredTx, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tx := range s.txs {
		if common.HexToHash(tx.Hash) == hash {
			return tx, true
		}
	}
	return StoredTx{}, false
}

func txInBlock(ctx context.Context, client *chain.Client, number uint64, hash common.Hash) (*types.Transaction, error) {
	block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(number))
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errBlockNil
	}
	for _, tx := range block.Transactions() {
		if tx.Hash() == hash {
			return tx, nil
		}
	}
	return nil, ErrUnknownTransaction
}

func revertReason(ctx context.Context, client *chain.Client, hash common.Hash) string {
	tr, err := client.TraceCall(ctx, hash)
	if err != nil || tr == nil {
		return "execution reverted"
	}
	return revertFromTrace(tr)
}

func revertFromTrace(tr *chain.CallTrace) string {
	if tr == nil {
		return "execution reverted"
	}
	if tr.RevertReason != "" {
		return tr.RevertReason
	}
	if s, err := abi.UnpackRevert(tr.Output); err == nil && s != "" {
		return s
	}
	if tr.Error != "" {
		return tr.Error
	}
	return "execution reverted"
}

func decodeCalldata(data []byte) []TxArg {
	if len(data) < 4 {
		return []TxArg{}
	}
	c, ok := contracts.BySelector(data[:4])
	if !ok || c == nil {
		return []TxArg{}
	}
	method, err := c.ABI.MethodById(data[:4])
	if err != nil || method == nil {
		return []TxArg{}
	}
	unpacked, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		return []TxArg{}
	}
	return argsFrom(method.Inputs, unpacked)
}

func decodeLog(lg *types.Log) TxLog {
	topics := make([]string, len(lg.Topics))
	for i, t := range lg.Topics {
		topics[i] = t.Hex()
	}
	out := TxLog{
		Address: lg.Address.Hex(),
		Topics:  topics,
		Data:    hexutil.Encode(lg.Data),
		Args:    []TxArg{},
	}
	if len(lg.Topics) == 0 {
		return out
	}
	c, ok := contracts.Get(lg.Address)
	if !ok || c == nil {
		return out
	}
	ev, err := c.ABI.EventByID(lg.Topics[0])
	if err != nil || ev == nil {
		return out
	}
	out.Name = ev.RawName
	out.Args = decodeEventArgs(*ev, lg)
	return out
}

func decodeEventArgs(ev abi.Event, lg *types.Log) []TxArg {
	nonIndexed := ev.Inputs.NonIndexed()
	unpacked, err := nonIndexed.Unpack(lg.Data)
	if err != nil {
		unpacked = nil
	}
	indexed := indexedArguments(ev.Inputs)
	idxMap := map[string]any{}
	if len(indexed) > 0 && len(lg.Topics) > 1 {
		_ = abi.ParseTopicsIntoMap(idxMap, indexed, lg.Topics[1:])
	}
	out := make([]TxArg, 0, len(ev.Inputs))
	nonI := 0
	for _, in := range ev.Inputs {
		arg := TxArg{Type: in.Type.String(), Name: in.Name}
		if in.Indexed {
			if v, ok := idxMap[in.Name]; ok {
				arg.Text = formatValue(v)
			}
		} else if nonI < len(unpacked) {
			arg.Text = formatValue(unpacked[nonI])
			nonI++
		}
		out = append(out, arg)
	}
	return out
}

func indexedArguments(args abi.Arguments) abi.Arguments {
	out := abi.Arguments{}
	for _, a := range args {
		if a.Indexed {
			out = append(out, a)
		}
	}
	return out
}

func argsFrom(inputs abi.Arguments, unpacked []any) []TxArg {
	out := make([]TxArg, 0, len(inputs))
	for i, in := range inputs {
		arg := TxArg{Type: in.Type.String(), Name: in.Name}
		if i < len(unpacked) {
			arg.Text = formatValue(unpacked[i])
		}
		out = append(out, arg)
	}
	return out
}

func balanceChangeList(stored StoredTx) []TxBalanceChange {
	if stored.BalanceChanges == nil {
		return []TxBalanceChange{}
	}
	out := make([]TxBalanceChange, 0, len(stored.BalanceChanges))
	seen := map[string]struct{}{}
	appendAddr := func(addr string) {
		if addr == "" {
			return
		}
		for k, v := range stored.BalanceChanges {
			if strings.EqualFold(k, addr) {
				key := strings.ToLower(k)
				if _, ok := seen[key]; ok {
					return
				}
				seen[key] = struct{}{}
				out = append(out, TxBalanceChange{Address: k, Delta: v})
				return
			}
		}
	}
	appendAddr(stored.From)
	appendAddr(stored.To)
	for k, v := range stored.BalanceChanges {
		key := strings.ToLower(k)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, TxBalanceChange{Address: k, Delta: v})
	}
	return out
}
