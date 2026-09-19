# Kabat

Kabat is a desktop developer wallet for local Ethereum work against [Anvil](https://book.getfoundry.sh/anvil/).

**Only use it for local smart contract development against Anvil.** Kabat ships Foundry’s well-known test accounts. Those keys are public. Do not point it at mainnet, a public testnet, or any chain with real funds.

## Features

- **MCP server** for coding agents: list wallets and contracts, send ETH or ERC-20, load ABIs, and call methods on your local Anvil node. See [AGENTS.md](AGENTS.md).
- Anvil wallets with names, balances, ETH sends, and ERC-20 tokens
- Transaction history with decoded calls
- Contract details, ABI loading, view calls, and writes
- Chrome and Firefox extension that injects an EIP-1193 provider (`window.ethereum`)

## Install

You need [Anvil](https://book.getfoundry.sh/getting-started/installation) running first:

```bash
anvil
```

Kabat talks to `http://127.0.0.1:8545` by default (change it in Settings).

### App

Download a build from [GitHub Releases](https://github.com/dszlachta/kabat/releases), or from the **Release** workflow artifacts.

- **macOS:** unzip `kabat-macos-arm64.zip` and open `kabat.app`
- **Linux:** run `kabat-linux-amd64` (Ubuntu 24.04+; needs GTK 4 and WebKitGTK 6)

To build from source you need Go 1.25+, Node 22+, and [Wails v3](https://v3.wails.io/quick-start/installation/). On Linux also install `libgtk-4-dev` and `libwebkitgtk-6.0-dev`. From the repo root, `go install` picks the Wails version in `go.mod`:

```bash
go install github.com/wailsapp/wails/v3/cmd/wails3
wails3 build
```

The binary lands in `bin/`.

### Browser extension (optional)

1. Load `extension/` as an unpacked Chrome or Firefox add-on.
2. Install the native messaging host so the extension can reach the app:

```bash
kabat install-native-host
```

Use the same `kabat` binary you run as the app.

### MCP (optional)

In Kabat, open Settings and enable **MCP server**. Agents connect to `http://127.0.0.1:19375`. Details for agents are in [AGENTS.md](AGENTS.md).
