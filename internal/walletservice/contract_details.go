package walletservice

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ErrUnknownContract is returned when address is not a successful create.
var ErrUnknownContract = errors.New("unknown contract")

// ContractDetails is the Contracts Details-tab payload.
type ContractDetails struct {
	Address     string        `json:"address"`
	Name        string        `json:"name,omitempty"`
	Deployer    string        `json:"deployer"`
	Hash        string        `json:"hash"`
	BlockNumber uint64        `json:"blockNumber"`
	Balance     string        `json:"balance"`
	CodeSize    int           `json:"codeSize"`
	Bytecode    string        `json:"bytecode"`
	ABILoaded   bool          `json:"abiLoaded"`
	Functions   int           `json:"functions"`
	Events      int           `json:"events"`
	Constructor []TxArg       `json:"constructorArgs"`
	ERC20       *ERC20Summary `json:"erc20,omitempty"`
}

// ContractDetails returns on-chain identity and deployment facts for address.
func (s *WalletService) ContractDetails(address string) (ContractDetails, error) {
	addr := common.HexToAddress(address)
	stored, ok := s.storedCreate(addr)
	if !ok {
		return ContractDetails{}, ErrUnknownContract
	}

	s.mu.Lock()
	url := s.url
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := chain.ConnectURL(ctx, url)
	if err != nil {
		return ContractDetails{}, err
	}
	defer client.Close()

	bal, err := client.BalanceAt(ctx, addr)
	if err != nil {
		return ContractDetails{}, err
	}
	code, err := client.CodeAt(ctx, addr)
	if err != nil {
		return ContractDetails{}, err
	}
	balance := "0x0"
	if bal != nil {
		balance = hexutil.EncodeBig(bal)
	}

	out := ContractDetails{
		Address:     stored.Contract,
		Deployer:    stored.From,
		Hash:        stored.Hash,
		BlockNumber: stored.BlockNumber,
		Balance:     balance,
		CodeSize:    len(code),
		Bytecode:    hexutil.Encode(code),
		Constructor: []TxArg{},
		ERC20:       probeERC20(ctx, client, addr, code),
	}
	s.applyStoredABI(ctx, client, addr)
	if c, ok := contracts.Get(addr); ok && c != nil {
		out.Name = c.Name
		out.ABILoaded = c.ABIJSON != ""
		out.Functions = len(c.ABI.Methods)
		out.Events = len(c.ABI.Events)
		if len(c.ABI.Constructor.Inputs) > 0 {
			if tx, err := txInBlock(ctx, client, stored.BlockNumber, common.HexToHash(stored.Hash)); err == nil && tx != nil {
				out.Constructor = decodeConstructor(c, code, tx.Data())
			}
		}
	}
	return out, nil
}

func (s *WalletService) storedCreate(addr common.Address) (StoredTx, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, tx := range s.txs {
		if tx.Kind != "create" || !tx.Ok || tx.Contract == "" {
			continue
		}
		if common.HexToAddress(tx.Contract) == addr {
			return tx, true
		}
	}
	return StoredTx{}, false
}

func decodeConstructor(c *contracts.Contract, runtime, deploy []byte) []TxArg {
	if c == nil {
		return []TxArg{}
	}
	inputs := c.ABI.Constructor.Inputs
	if len(inputs) == 0 {
		return []TxArg{}
	}
	unpacked, err := inputs.Unpack(constructorArgData(runtime, deploy))
	if err != nil {
		return []TxArg{}
	}
	return argsFrom(inputs, unpacked)
}

func constructorArgData(runtime, deploy []byte) []byte {
	if len(runtime) == 0 || len(deploy) == 0 {
		return deploy
	}
	i := bytes.LastIndex(deploy, runtime)
	if i < 0 {
		return deploy
	}
	return deploy[i+len(runtime):]
}
