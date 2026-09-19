package provider

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type rpcDataErr struct {
	msg  string
	code int
	data any
}

func (e *rpcDataErr) Error() string          { return e.msg }
func (e *rpcDataErr) ErrorCode() int         { return e.code }
func (e *rpcDataErr) ErrorData() interface{} { return e.data }

func TestWrapError_copiesMessageAndData(t *testing.T) {
	err := wrapError(&rpcDataErr{
		msg:  "execution reverted",
		code: -32000,
		data: "0x08c379a0",
	})
	var rpcErr *ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, -32000, rpcErr.Code)
	require.Equal(t, "execution reverted", rpcErr.Message)
	require.Equal(t, "0x08c379a0", rpcErr.Data)
}

func TestWrapError_plainError(t *testing.T) {
	err := wrapError(errors.New("dial refused"))
	var rpcErr *ProviderRpcError
	require.ErrorAs(t, err, &rpcErr)
	require.Equal(t, codeDisconnected, rpcErr.Code)
	require.Equal(t, "dial refused", rpcErr.Message)
	require.Nil(t, rpcErr.Data)
}
