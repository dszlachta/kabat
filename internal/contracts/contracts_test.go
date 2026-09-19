package contracts

import (
	"context"
	"math/big"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

type web3API struct{}

func (web3API) ClientVersion() string { return "anvil/v1.0.0" }

type ethAPI struct {
	codes map[common.Address][]byte
}

func (a *ethAPI) ChainId() *hexutil.Big {
	return (*hexutil.Big)(big.NewInt(31337))
}

func (a *ethAPI) GetCode(_ context.Context, address common.Address, _ rpc.BlockNumberOrHash) hexutil.Bytes {
	if a.codes == nil {
		return nil
	}
	return a.codes[address]
}

func connect(t *testing.T, eth *ethAPI) *chain.Client {
	t.Helper()
	srv := rpc.NewServer()
	t.Cleanup(srv.Stop)
	require.NoError(t, srv.RegisterName("web3", web3API{}))
	require.NoError(t, srv.RegisterName("eth", eth))
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	client, err := chain.ConnectURL(context.Background(), httpSrv.URL)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return client
}

func TestMain(m *testing.M) {
	m.Run()
	reset()
}

func TestNewAndGet(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x1111111111111111111111111111111111111111")
	code := []byte{0x60, 0x00, 0x60, 0x00, 0xfd}
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: code}})

	got, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.Equal(t, addr, got.Address)
	require.Equal(t, code, got.Bytecode)

	stored, ok := Get(addr)
	require.True(t, ok)
	require.Equal(t, got, stored)
}

func TestNew_emptyCode(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x2222222222222222222222222222222222222222")
	client := connect(t, &ethAPI{})
	_, err := New(context.Background(), client, addr)
	require.ErrorIs(t, err, ErrNotContract)
	_, ok := Get(addr)
	require.False(t, ok)
}

func TestNew_refreshesBytecodeKeepsNameAndABI(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x3333333333333333333333333333333333333333")
	first := []byte{0x01}
	second := []byte{0x02, 0x03}
	eth := &ethAPI{codes: map[common.Address][]byte{addr: first}}
	client := connect(t, eth)

	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	c.SetName("Token")
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "abi.json")))

	eth.codes[addr] = second
	again, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.Equal(t, c, again)
	require.Equal(t, second, again.Bytecode)
	require.Equal(t, "Token", again.Name)
	_, ok := again.ABI.Methods["name"]
	require.True(t, ok)
	require.Contains(t, again.ABIJSON, `"name"`)
}

func TestLoadABI_arrayAndArtifact(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0x4444444444444444444444444444444444444444")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)

	require.NoError(t, c.LoadABI(filepath.Join("testdata", "abi.json")))
	_, ok := c.ABI.Methods["name"]
	require.True(t, ok)
	require.Contains(t, c.ABIJSON, `"name"`)
	require.Contains(t, c.ABIJSON, "\n  ")
	nameID := c.ABI.Methods["name"].ID
	indexed, found := BySelector(nameID)
	require.True(t, found)
	require.Equal(t, c, indexed)

	require.NoError(t, c.LoadABI(filepath.Join("testdata", "artifact.json")))
	_, ok = c.ABI.Methods["symbol"]
	require.True(t, ok)
	_, ok = c.ABI.Methods["name"]
	require.False(t, ok)
	require.Contains(t, c.ABIJSON, `"symbol"`)
	_, found = BySelector(nameID)
	require.False(t, found)
	symbolID := c.ABI.Methods["symbol"].ID
	indexed, found = BySelector(symbolID)
	require.True(t, found)
	require.Equal(t, c, indexed)
}

func TestLoadABI_weth9Artifact(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0xc02aaa39b223fe8d0a0e5c4f27ead9083c756cc2")
	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "weth9.json")))
	_, ok := c.ABI.Methods["deposit"]
	require.True(t, ok)
	_, ok = c.ABI.Methods["withdraw"]
	require.True(t, ok)
	_, ok = c.ABI.Methods["transfer"]
	require.True(t, ok)
	_, ok = c.ABI.Methods["balanceOf"]
	require.True(t, ok)
	_, ok = c.ABI.Events["Transfer"]
	require.True(t, ok)
}

func TestMethodName(t *testing.T) {
	reset()
	t.Cleanup(reset)

	addr := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.Empty(t, MethodName(addr, []byte{0x70, 0xa0, 0x82, 0x31}))

	client := connect(t, &ethAPI{codes: map[common.Address][]byte{addr: {0xff}}})
	c, err := New(context.Background(), client, addr)
	require.NoError(t, err)
	require.NoError(t, c.LoadABI(filepath.Join("testdata", "abi_members.json")))

	require.Equal(t, "balanceOf", MethodName(addr, []byte{0x70, 0xa0, 0x82, 0x31}))
	require.Equal(t, "transfer", MethodName(addr, []byte{0xa9, 0x05, 0x9c, 0xbb}))
	got, ok := BySelector([]byte{0x70, 0xa0, 0x82, 0x31})
	require.True(t, ok)
	require.Equal(t, addr, got.Address)
	require.Equal(t, "balanceOf", MethodBySelector([]byte{0x70, 0xa0, 0x82, 0x31}))
	names := SelectorNames()
	require.Equal(t, "balanceOf", names["0x70a08231"])
	require.Equal(t, "transfer", names["0xa9059cbb"])
	require.Empty(t, MethodName(addr, []byte{0xde, 0xad, 0xbe, 0xef}))
	require.Empty(t, MethodName(addr, []byte{0x70, 0xa0}))
}
