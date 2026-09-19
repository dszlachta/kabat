package provider

import (
	"context"
	"math/big"

	"github.com/dszlachta/kabat/internal/eip712"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

func (p *Provider) signTypedData(ctx context.Context, params any) (any, error) {
	from, typed, raw, err := eip712.Parse(params)
	if err != nil {
		return nil, &ProviderRpcError{Code: -32602, Message: "invalid typed data params"}
	}
	if _, ok := wallets.Find(from); !ok || from != p.selected {
		return nil, &ProviderRpcError{Code: codeUnauthorized, Message: "Unauthorized"}
	}
	if typed.Domain.ChainId != nil {
		chainID, err := p.chain.ChainID(ctx)
		if err != nil {
			return nil, wrapError(err)
		}
		if (*big.Int)(typed.Domain.ChainId).Cmp(chainID) != 0 {
			return nil, &ProviderRpcError{Code: -32602, Message: "invalid typed data params"}
		}
	}

	view, err := eip712.Display(typed, raw)
	if err != nil {
		return nil, &ProviderRpcError{Code: -32602, Message: "invalid typed data params"}
	}

	approved := false
	if p.confirm != nil {
		approved, err = p.confirm.Confirm(ctx, TransactionRequest{
			From: from,
			Raw:  view,
		})
		if err != nil {
			return nil, wrapError(err)
		}
	}
	if !approved {
		return nil, &ProviderRpcError{Code: codeUserRejected, Message: "User Rejected Request"}
	}

	account, ok := wallets.Find(from)
	if !ok {
		return nil, &ProviderRpcError{Code: codeUnauthorized, Message: "Unauthorized"}
	}
	digest, err := eip712.Hash(typed)
	if err != nil {
		return nil, &ProviderRpcError{Code: -32602, Message: "invalid typed data params"}
	}
	sig, err := crypto.Sign(digest, account.PrivateKey)
	if err != nil {
		return nil, wrapError(err)
	}
	sig[64] += 27
	return hexutil.Encode(sig), nil
}
