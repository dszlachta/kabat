package mcpserver_test

import (
	"context"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/dszlachta/kabat/internal/mcpserver"
	"github.com/dszlachta/kabat/internal/wallets"
	"github.com/dszlachta/kabat/internal/walletservice"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestHandler_listsUserTools(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	ts := httptest.NewServer(mcpserver.Handler(svc))
	t.Cleanup(ts.Close)

	session := mcpSession(t, ts.URL)
	t.Cleanup(func() { _ = session.Close() })

	got, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(got.Tools))
	for _, tool := range got.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	require.Equal(t, []string{
		"abi_selectors",
		"add_random_wallet",
		"add_wallet_from_key",
		"allow_connection",
		"call_contract",
		"cancel_transaction",
		"confirm_transaction",
		"contract_details",
		"contract_info",
		"dapp_messages",
		"deny_connection",
		"disconnect_session",
		"get_settings",
		"list_accounts",
		"list_contracts",
		"list_tokens",
		"list_transactions",
		"list_transactions_for",
		"load_contract_abi",
		"select_account",
		"selected_account",
		"send_erc20",
		"send_eth",
		"set_account_name",
		"set_anvil_url",
		"set_mcp",
		"transaction_details",
	}, names)
}

func TestHandler_selectedAccount(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	ts := httptest.NewServer(mcpserver.Handler(svc))
	t.Cleanup(ts.Close)
	session := mcpSession(t, ts.URL)
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "selected_account",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Contains(t, toolText(t, res), wallets.Addresses()[0])
}

func TestHandler_selectAccount(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	ts := httptest.NewServer(mcpserver.Handler(svc))
	t.Cleanup(ts.Close)
	session := mcpSession(t, ts.URL)
	t.Cleanup(func() { _ = session.Close() })

	want := wallets.Addresses()[2]
	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "select_account",
		Arguments: map[string]any{"address": want},
	})
	require.NoError(t, err)
	require.Equal(t, want, svc.Selected())
}

func TestStartStop_loopback(t *testing.T) {
	svc := walletservice.NewWithURL("http://127.0.0.1:1")
	srv := mcpserver.New(svc)
	require.NoError(t, srv.StartOn("127.0.0.1:0"))
	require.NoError(t, srv.SetEnabled(false))
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, srv.SetEnabled(false))
}

func mcpSession(t *testing.T, url string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "kabat-test", Version: "v1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: url}, nil)
	require.NoError(t, err)
	return session
}

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, res.Content)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	return text.Text
}
