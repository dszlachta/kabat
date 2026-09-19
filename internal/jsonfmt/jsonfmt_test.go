package jsonfmt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBytes_indentsObject(t *testing.T) {
	got := Bytes([]byte(`{"a":1,"b":[2,3]}`))
	require.Equal(t, "{\n  \"a\": 1,\n  \"b\": [\n    2,\n    3\n  ]\n}", got)
}

func TestBytes_invalidUnchanged(t *testing.T) {
	require.Equal(t, "{", Bytes([]byte(`{`)))
}

func TestValue_nilEmpty(t *testing.T) {
	require.Empty(t, Value(nil))
}

func TestValue_slice(t *testing.T) {
	require.Equal(t, "[\n  1,\n  2\n]", Value([]any{1, 2}))
}
