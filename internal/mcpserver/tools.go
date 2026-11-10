package mcpserver

import (
	"context"

	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addTools(server *mcp.Server, svc *walletservice.WalletService) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_accounts",
		Description: "List Anvil wallets with names, balances, selection, and dapp connections",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, accountsOut, error) {
		accs, err := svc.Accounts()
		if err != nil {
			return nil, accountsOut{}, err
		}
		if accs == nil {
			accs = []walletservice.Account{}
		}
		return nil, accountsOut{Accounts: accs}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "selected_account",
		Description: "Checksummed address of the UI-selected wallet",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, selectedOut, error) {
		return nil, selectedOut{Address: svc.Selected()}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "select_account",
		Description: "Select a wallet in the UI (does not change dapp sessions)",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addressIn) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.Select(in.Address)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_account_name",
		Description: "Set or clear a wallet display name",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in nameIn) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.SetAccountName(in.Address, in.Name)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_random_wallet",
		Description: "Generate a new wallet, add it, and select it",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.AddRandomWallet()
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_wallet_from_key",
		Description: "Import a hex private key (optional 0x), add it, and select it",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in keyIn) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.AddWalletFromKey(in.PrivateKey)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_eth",
		Description: "Send ETH from a wallet. Value is wei hex (0x…). Empty from uses the selected wallet. No confirm dialog.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in sendIn) (*mcp.CallToolResult, hashOut, error) {
		hash, err := svc.SendETHAuto(in.From, in.To, in.Value)
		if err != nil {
			return nil, hashOut{}, err
		}
		return nil, hashOut{Hash: hash}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tokens",
		Description: "ERC-20 tokens among contracts created by Anvil wallets",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, tokensOut, error) {
		got, err := svc.Tokens()
		if err != nil {
			return nil, tokensOut{}, err
		}
		if got == nil {
			got = []walletservice.ERC20Token{}
		}
		return nil, tokensOut{Tokens: got}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "send_erc20",
		Description: "Send an ERC-20 transfer. Amount is a decimal string in token units. Empty from uses the selected wallet. No confirm dialog.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in sendERC20In) (*mcp.CallToolResult, hashOut, error) {
		hash, err := svc.SendERC20Auto(in.From, in.Token, in.To, in.Amount)
		if err != nil {
			return nil, hashOut{}, err
		}
		return nil, hashOut{Hash: hash}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "disconnect_session",
		Description: "Disconnect a dapp session so later RPCs from it are unauthorized",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in sessionIn) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.Disconnect(in.SessionUlid)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "dapp_messages",
		Description: "In-memory dapp RPC log (newest last)",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, messagesOut, error) {
		return nil, messagesOut{Messages: svc.DappMessages()}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_transactions",
		Description: "Mined transactions, newest first",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, txsOut, error) {
		return nil, txsOut{Transactions: svc.Transactions()}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_transactions_for",
		Description: "Mined transactions involving an address (from or to), newest first",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addressIn) (*mcp.CallToolResult, txsOut, error) {
		return nil, txsOut{Transactions: svc.TransactionsFor(in.Address)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "transaction_details",
		Description: "Decoded details for a mined transaction",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in hashIn) (*mcp.CallToolResult, walletservice.TxDetails, error) {
		got, err := svc.TxDetails(in.Hash)
		if err != nil {
			return nil, walletservice.TxDetails{}, err
		}
		return nil, got, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_contracts",
		Description: "Contracts created by Anvil wallets, newest first",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, contractsOut, error) {
		return nil, contractsOut{Contracts: svc.Contracts()}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contract_details",
		Description: "On-chain identity and deployment facts for a contract address",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addressIn) (*mcp.CallToolResult, walletservice.ContractDetails, error) {
		got, err := svc.ContractDetails(in.Address)
		if err != nil {
			return nil, walletservice.ContractDetails{}, err
		}
		return nil, got, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "contract_info",
		Description: "Loaded ABI, functions, and events for a contract address",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addressIn) (*mcp.CallToolResult, walletservice.ContractInfo, error) {
		return nil, svc.ContractInfo(in.Address), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "load_contract_abi",
		Description: "Load an ABI JSON file from a local path for a contract address",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in abiIn) (*mcp.CallToolResult, walletservice.ContractInfo, error) {
		got, err := svc.LoadContractABIFromPath(in.Address, in.Path)
		if err != nil {
			return nil, walletservice.ContractInfo{}, err
		}
		return nil, got, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "call_contract",
		Description: "eth_call a view/pure ABI method, or send a tx. Selector is 0x… function id. Value is wei hex. Empty from uses the selected wallet. Sends skip the confirm dialog.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in callIn) (*mcp.CallToolResult, walletservice.ContractCallResult, error) {
		got, err := svc.CallContractAuto(in.Address, in.Selector, in.Args, in.From, in.Value)
		if err != nil {
			return nil, walletservice.ContractCallResult{}, err
		}
		return nil, got, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "abi_selectors",
		Description: "Map of 4-byte selector hex to function name from loaded ABIs",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, selectorsOut, error) {
		return nil, selectorsOut{Selectors: svc.ABISelectors()}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_settings",
		Description: "Anvil URL, MCP toggle, and MCP listen URL",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, walletservice.Settings, error) {
		return nil, svc.Settings(), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_anvil_url",
		Description: "Set the Anvil JSON-RPC URL. Empty resets to the default.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in urlIn) (*mcp.CallToolResult, walletservice.Settings, error) {
		if err := svc.SetAnvilURL(in.URL); err != nil {
			return nil, walletservice.Settings{}, err
		}
		return nil, svc.Settings(), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "set_mcp",
		Description: "Enable or disable the MCP server. Disabling stops this server.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in mcpIn) (*mcp.CallToolResult, walletservice.Settings, error) {
		if err := svc.SetMCP(in.Enabled); err != nil {
			return nil, walletservice.Settings{}, err
		}
		return nil, svc.Settings(), nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "confirm_transaction",
		Description: "Approve the pending transaction confirm dialog",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.ConfirmPending()
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "cancel_transaction",
		Description: "Reject the pending transaction confirm dialog",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.CancelPending()
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "allow_connection",
		Description: "Approve the pending dapp connection for a wallet address",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in addressIn) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.AllowConnect(in.Address)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "deny_connection",
		Description: "Reject the pending dapp connection dialog",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, struct{}, error) {
		return nil, struct{}{}, svc.CancelConnect()
	})
}

type accountsOut struct {
	Accounts []walletservice.Account `json:"accounts"`
}

type selectedOut struct {
	Address string `json:"address"`
}

type addressIn struct {
	Address string `json:"address" jsonschema:"wallet or contract address"`
}

type nameIn struct {
	Address string `json:"address" jsonschema:"wallet address"`
	Name    string `json:"name" jsonschema:"display name; empty clears it"`
}

type keyIn struct {
	PrivateKey string `json:"privateKey" jsonschema:"hex private key, optional 0x prefix"`
}

type sendIn struct {
	To    string `json:"to" jsonschema:"recipient address"`
	Value string `json:"value" jsonschema:"amount in wei hex, e.g. 0xde0b6b3a7640000"`
	From  string `json:"from,omitempty" jsonschema:"sender address; empty uses the selected wallet"`
}

type sendERC20In struct {
	Token  string `json:"token" jsonschema:"ERC-20 contract address"`
	To     string `json:"to" jsonschema:"recipient address"`
	Amount string `json:"amount" jsonschema:"decimal amount in token units, e.g. 1.5"`
	From   string `json:"from,omitempty" jsonschema:"sender address; empty uses the selected wallet"`
}

type tokensOut struct {
	Tokens []walletservice.ERC20Token `json:"tokens"`
}

type sessionIn struct {
	SessionUlid string `json:"sessionUlid" jsonschema:"dapp session ULID"`
}

type messagesOut struct {
	Messages []walletservice.DappMessage `json:"messages"`
}

type txsOut struct {
	Transactions []walletservice.StoredTx `json:"transactions"`
}

type hashIn struct {
	Hash string `json:"hash" jsonschema:"transaction hash"`
}

type hashOut struct {
	Hash string `json:"hash"`
}

type contractsOut struct {
	Contracts []walletservice.StoredContract `json:"contracts"`
}

type abiIn struct {
	Address string `json:"address" jsonschema:"contract address"`
	Path    string `json:"path" jsonschema:"absolute path to ABI JSON"`
}

type callIn struct {
	Address  string   `json:"address" jsonschema:"contract address"`
	Selector string   `json:"selector" jsonschema:"0x-prefixed 4-byte function selector"`
	Args     []string `json:"args,omitempty" jsonschema:"ABI argument strings in order"`
	From     string   `json:"from,omitempty" jsonschema:"sender; empty uses the selected wallet"`
	Value    string   `json:"value,omitempty" jsonschema:"wei hex; empty is 0"`
}

type selectorsOut struct {
	Selectors map[string]string `json:"selectors"`
}

type urlIn struct {
	URL string `json:"url" jsonschema:"Anvil JSON-RPC URL; empty resets to default"`
}

type mcpIn struct {
	Enabled bool `json:"enabled" jsonschema:"true starts the MCP server, false stops it"`
}
