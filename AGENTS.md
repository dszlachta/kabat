# Kabat MCP

Kabat is a local Anvil developer wallet. When the app is running and **MCP server** is enabled in Settings, it serves [Model Context Protocol](https://modelcontextprotocol.io/) tools on loopback so you can inspect wallets, send, and call contracts.

**Anvil only.** Do not use these tools against mainnet, a public testnet, or any chain with real funds. Kabat’s default accounts are Foundry’s well-known keys.

## Connect

1. Start Anvil (`anvil`, default `http://127.0.0.1:8545`).
2. Start Kabat.
3. Enable **MCP server** in Settings.

Endpoint:

```
http://127.0.0.1:19375
```

Transport is **streamable HTTP** with JSON responses. The process is Kabat itself; do not spawn a second server.

Cursor example (`~/.cursor/mcp.json` or project `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "kabat": {
      "url": "http://127.0.0.1:19375"
    }
  }
}
```

If the tools are missing, Kabat is not running or MCP is off.

## Tools

Writes from MCP skip the in-app confirm dialog.

| Tool | Notes |
| --- | --- |
| `list_accounts` | Wallets, names, balances, selection, dapp sessions |
| `selected_account` | UI-selected wallet |
| `select_account` | UI selection only; does not change dapp sessions |
| `set_account_name` | Empty name clears it |
| `add_random_wallet` | Creates and selects |
| `add_wallet_from_key` | Hex key, optional `0x` |
| `send_eth` | `value` is **wei hex** (`0x…`). Empty `from` = selected wallet |
| `list_tokens` | ERC-20s among contracts created by Anvil wallets |
| `send_erc20` | `amount` is a **decimal string in token units**. Empty `from` = selected wallet |
| `list_transactions` / `list_transactions_for` | Newest first |
| `transaction_details` | Decoded mined tx |
| `list_contracts` / `contract_details` / `contract_info` | Created by Anvil wallets; `contract_info` is the loaded ABI |
| `load_contract_abi` | Local filesystem path to ABI JSON |
| `call_contract` | View/pure `eth_call`, or a send. `selector` is `0x` + 4 bytes. `value` is wei hex. Empty `from` = selected wallet |
| `abi_selectors` | Loaded 4-byte selector → name |
| `dapp_messages` | In-memory RPC log |
| `disconnect_session` | `sessionUlid` is the dapp connection key |
| `get_settings` / `set_anvil_url` / `set_mcp` | `set_mcp(false)` stops this server |
| `confirm_transaction` / `cancel_transaction` | Pending **dapp** confirm dialog (not MCP sends) |
| `allow_connection` / `deny_connection` | Pending dapp connect dialog |

## Defaults

- Anvil JSON-RPC: `http://127.0.0.1:8545`
- MCP: `http://127.0.0.1:19375`
- `send_eth` / `call_contract` amounts are wei hex; `send_erc20` amounts are token decimals
- `personal_sign` is unsupported
