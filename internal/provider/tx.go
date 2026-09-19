package provider

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	codeUserRejected = 4001
	codeUnauthorized = 4100
)

// Confirmer asks the user to approve a transaction.
type Confirmer interface {
	Confirm(ctx context.Context, tx TransactionRequest) (bool, error)
}

// TransactionRequest is an eth_sendTransaction payload.
type TransactionRequest struct {
	From                 common.Address
	To                   *common.Address
	Gas                  *uint64
	GasPrice             *big.Int
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
	Value                *big.Int
	Data                 []byte
	Nonce                *uint64
	Raw                  map[string]any
}

type rpcTransaction struct {
	From                 string          `json:"from"`
	To                   string          `json:"to"`
	Gas                  *hexutil.Uint64 `json:"gas"`
	GasPrice             *hexutil.Big    `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big    `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big    `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big    `json:"value"`
	Data                 *hexutil.Bytes  `json:"data"`
	Input                *hexutil.Bytes  `json:"input"`
	Nonce                *hexutil.Uint64 `json:"nonce"`
}

func (p *Provider) sendTransaction(ctx context.Context, params any) (any, error) {
	tx, err := parseSendTx(params)
	if err != nil {
		return nil, err
	}
	if tx.From == (common.Address{}) {
		tx.From = p.selected
	}
	if _, ok := wallets.Find(tx.From); !ok || tx.From != p.selected {
		return nil, &ProviderRpcError{Code: codeUnauthorized, Message: "Unauthorized"}
	}

	approved := false
	if p.confirm != nil {
		approved, err = p.confirm.Confirm(ctx, tx)
		if err != nil {
			return nil, wrapError(err)
		}
	}
	if !approved {
		return nil, &ProviderRpcError{Code: codeUserRejected, Message: "User Rejected Request"}
	}

	hash, err := p.signAndSend(ctx, tx)
	if err != nil {
		return nil, wrapError(err)
	}
	return hash, nil
}

func parseSendTx(params any) (TransactionRequest, error) {
	invalid := &ProviderRpcError{Code: -32602, Message: "invalid transaction params"}
	b, err := json.Marshal(params)
	if err != nil {
		return TransactionRequest{}, invalid
	}

	var rawList []map[string]any
	// Best-effort original JSON object; typed unmarshal below is the real parse.
	_ = json.Unmarshal(b, &rawList)
	var list []rpcTransaction
	if err := json.Unmarshal(b, &list); err != nil || len(list) == 0 {
		var single rpcTransaction
		if err := json.Unmarshal(b, &single); err != nil {
			return TransactionRequest{}, invalid
		}
		list = []rpcTransaction{single}
		var raw map[string]any
		if json.Unmarshal(b, &raw) == nil {
			rawList = []map[string]any{raw}
		}
	}
	rpcTx := list[0]
	raw := map[string]any{}
	if len(rawList) > 0 {
		raw = rawList[0]
	}

	tx := TransactionRequest{Raw: raw}
	if rpcTx.From != "" {
		tx.From = common.HexToAddress(rpcTx.From)
	}
	if rpcTx.To != "" {
		to := common.HexToAddress(rpcTx.To)
		tx.To = &to
	}
	if rpcTx.Gas != nil {
		gas := uint64(*rpcTx.Gas)
		tx.Gas = &gas
	}
	if rpcTx.GasPrice != nil {
		tx.GasPrice = (*big.Int)(rpcTx.GasPrice)
	}
	if rpcTx.MaxFeePerGas != nil {
		tx.MaxFeePerGas = (*big.Int)(rpcTx.MaxFeePerGas)
	}
	if rpcTx.MaxPriorityFeePerGas != nil {
		tx.MaxPriorityFeePerGas = (*big.Int)(rpcTx.MaxPriorityFeePerGas)
	}
	if rpcTx.Value != nil {
		tx.Value = (*big.Int)(rpcTx.Value)
	}
	if rpcTx.Data != nil {
		tx.Data = []byte(*rpcTx.Data)
	} else if rpcTx.Input != nil {
		tx.Data = []byte(*rpcTx.Input)
	}
	if rpcTx.Nonce != nil {
		nonce := uint64(*rpcTx.Nonce)
		tx.Nonce = &nonce
	}
	return tx, nil
}

func (p *Provider) signAndSend(ctx context.Context, tx TransactionRequest) (string, error) {
	account, ok := wallets.Find(tx.From)
	if !ok {
		return "", &ProviderRpcError{Code: codeUnauthorized, Message: "Unauthorized"}
	}
	eth := p.chain.Eth()
	chainID, err := eth.ChainID(ctx)
	if err != nil {
		return "", err
	}

	nonce := uint64(0)
	if tx.Nonce != nil {
		nonce = *tx.Nonce
	} else {
		nonce, err = eth.PendingNonceAt(ctx, tx.From)
		if err != nil {
			return "", err
		}
	}

	value := tx.Value
	if value == nil {
		value = big.NewInt(0)
	}

	gas := uint64(0)
	if tx.Gas != nil {
		gas = *tx.Gas
	} else {
		gas, err = eth.EstimateGas(ctx, ethereum.CallMsg{
			From:  tx.From,
			To:    tx.To,
			Value: value,
			Data:  tx.Data,
		})
		if err != nil {
			return "", err
		}
	}

	var inner types.TxData
	if tx.MaxFeePerGas != nil {
		tip := tx.MaxPriorityFeePerGas
		if tip == nil {
			tip, err = eth.SuggestGasTipCap(ctx)
			if err != nil {
				return "", err
			}
		}
		inner = &types.DynamicFeeTx{
			ChainID:   chainID,
			Nonce:     nonce,
			GasTipCap: tip,
			GasFeeCap: tx.MaxFeePerGas,
			Gas:       gas,
			To:        tx.To,
			Value:     value,
			Data:      tx.Data,
		}
	} else {
		gasPrice := tx.GasPrice
		if gasPrice == nil {
			gasPrice, err = eth.SuggestGasPrice(ctx)
			if err != nil {
				return "", err
			}
		}
		inner = &types.LegacyTx{
			Nonce:    nonce,
			GasPrice: gasPrice,
			Gas:      gas,
			To:       tx.To,
			Value:    value,
			Data:     tx.Data,
		}
	}

	signed, err := types.SignTx(types.NewTx(inner), types.LatestSignerForChainID(chainID), account.PrivateKey)
	if err != nil {
		return "", err
	}
	if err := eth.SendTransaction(ctx, signed); err != nil {
		return "", err
	}
	return signed.Hash().Hex(), nil
}
