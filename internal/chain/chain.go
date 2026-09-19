package chain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const DefaultURL = "http://127.0.0.1:8545"

var ErrAnvilUnavailable = errors.New("cannot connect to local Anvil chain")

// Client is a connection to a local Anvil node.
type Client struct {
	eth *ethclient.Client
}

// Connect dials the default Anvil endpoint at localhost:8545.
func Connect(ctx context.Context) (*Client, error) {
	return ConnectURL(ctx, DefaultURL)
}

// ConnectURL dials url and verifies the node identifies as Anvil.
func ConnectURL(ctx context.Context, url string) (*Client, error) {
	eth, err := ethclient.DialContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAnvilUnavailable, err)
	}
	var version string
	if err := eth.Client().CallContext(ctx, &version, "web3_clientVersion"); err != nil {
		eth.Close()
		return nil, fmt.Errorf("%w: %v", ErrAnvilUnavailable, err)
	}
	if !strings.Contains(strings.ToLower(version), "anvil") {
		eth.Close()
		return nil, ErrAnvilUnavailable
	}
	return &Client{eth: eth}, nil
}

// Eth returns the underlying go-ethereum ethclient.
func (c *Client) Eth() *ethclient.Client {
	return c.eth
}

// RPC returns the underlying go-ethereum RPC client.
func (c *Client) RPC() *rpc.Client {
	return c.eth.Client()
}

// BalanceAt returns the latest wei balance for addr.
func (c *Client) BalanceAt(ctx context.Context, addr common.Address) (*big.Int, error) {
	return c.eth.BalanceAt(ctx, addr, nil)
}

// BalancesAt returns latest wei balances for addrs in a single JSON-RPC batch.
func (c *Client) BalancesAt(ctx context.Context, addrs []common.Address) ([]*big.Int, error) {
	if len(addrs) == 0 {
		return nil, nil
	}
	elems := make([]rpc.BatchElem, len(addrs))
	results := make([]hexutil.Big, len(addrs))
	for i, addr := range addrs {
		elems[i] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args:   []any{addr, "latest"},
			Result: &results[i],
		}
	}
	if err := c.RPC().BatchCallContext(ctx, elems); err != nil {
		return nil, err
	}
	out := make([]*big.Int, len(addrs))
	for i, el := range elems {
		if el.Error != nil {
			return nil, el.Error
		}
		out[i] = new(big.Int).Set((*big.Int)(&results[i]))
	}
	return out, nil
}

// WebsocketURL maps an HTTP JSON-RPC URL to the equivalent WebSocket URL.
func WebsocketURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	return u.String()
}

// SubscribeNewHeads is eth_subscribe "newHeads".
// Anvil-only: notifications require the local Foundry WebSocket, not HTTP JSON-RPC.
func (c *Client) SubscribeNewHeads(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	return c.eth.SubscribeNewHead(ctx, ch)
}

// ChainID returns the connected chain id.
func (c *Client) ChainID(ctx context.Context) (*big.Int, error) {
	return c.eth.ChainID(ctx)
}

// BlockNumber returns the latest block number.
func (c *Client) BlockNumber(ctx context.Context) (uint64, error) {
	return c.eth.BlockNumber(ctx)
}

// BlockByNumber returns the block with full transactions.
func (c *Client) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return c.eth.BlockByNumber(ctx, number)
}

// BlockByHash returns the block with full transactions.
func (c *Client) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return c.eth.BlockByHash(ctx, hash)
}

// TransactionReceipt returns the receipt for hash.
func (c *Client) TransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
	return c.eth.TransactionReceipt(ctx, hash)
}

// CallTrace is debug_traceTransaction with callTracer.
type CallTrace struct {
	Error        string        `json:"error,omitempty"`
	RevertReason string        `json:"revertReason,omitempty"`
	Output       hexutil.Bytes `json:"output,omitempty"`
}

// TraceCall is debug_traceTransaction using callTracer.
func (c *Client) TraceCall(ctx context.Context, hash common.Hash) (*CallTrace, error) {
	var out CallTrace
	err := c.RPC().CallContext(ctx, &out, "debug_traceTransaction", hash, map[string]any{
		"tracer": "callTracer",
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// CodeAt returns the latest bytecode at addr (empty if none).
func (c *Client) CodeAt(ctx context.Context, addr common.Address) ([]byte, error) {
	return c.eth.CodeAt(ctx, addr, nil)
}

// Call is eth_call against the latest block.
func (c *Client) Call(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	return c.eth.CallContract(ctx, msg, nil)
}

// TraceAccount is the balance fields from prestateTracer (diffMode).
type TraceAccount struct {
	Balance *hexutil.Big `json:"balance,omitempty"`
}

// TxDiff is pre/post account balances for one transaction.
type TxDiff struct {
	Pre  map[common.Address]TraceAccount `json:"pre"`
	Post map[common.Address]TraceAccount `json:"post"`
}

// Balance returns pre and post ETH for addr. Missing side is 0; a post account
// with no balance field keeps the pre balance (field-level diff).
func (d *TxDiff) Balance(addr common.Address) (pre, post *big.Int) {
	pre = new(big.Int)
	post = new(big.Int)
	if d == nil {
		return pre, post
	}
	preAcc, inPre := d.Pre[addr]
	postAcc, inPost := d.Post[addr]
	if inPre && preAcc.Balance != nil {
		pre = (*big.Int)(preAcc.Balance)
	}
	switch {
	case inPost && postAcc.Balance != nil:
		post = (*big.Int)(postAcc.Balance)
	case inPost:
		post = new(big.Int).Set(pre)
	}
	return pre, post
}

// TraceTxDiff is debug_traceTransaction with prestateTracer in diffMode.
func (c *Client) TraceTxDiff(ctx context.Context, hash common.Hash) (*TxDiff, error) {
	var out TxDiff
	err := c.RPC().CallContext(ctx, &out, "debug_traceTransaction", hash, map[string]any{
		"tracer":       "prestateTracer",
		"tracerConfig": map[string]any{"diffMode": true},
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Close closes the RPC connection.
func (c *Client) Close() {
	c.eth.Close()
}
