package eip712

import (
	"encoding/json"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

// Canonical EIP-712 Ether Mail example.
var etherMail = map[string]any{
	"types": map[string]any{
		"EIP712Domain": []any{
			map[string]any{"name": "name", "type": "string"},
			map[string]any{"name": "version", "type": "string"},
			map[string]any{"name": "chainId", "type": "uint256"},
			map[string]any{"name": "verifyingContract", "type": "address"},
		},
		"Person": []any{
			map[string]any{"name": "name", "type": "string"},
			map[string]any{"name": "wallet", "type": "address"},
		},
		"Mail": []any{
			map[string]any{"name": "from", "type": "Person"},
			map[string]any{"name": "to", "type": "Person"},
			map[string]any{"name": "contents", "type": "string"},
		},
	},
	"primaryType": "Mail",
	"domain": map[string]any{
		"name":              "Ether Mail",
		"version":           "1",
		"chainId":           1,
		"verifyingContract": "0xCcCCccccCCCCcCCCCCCcCcCccCcCCCcCcccccccC",
	},
	"message": map[string]any{
		"from": map[string]any{
			"name":   "Cow",
			"wallet": "0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826",
		},
		"to": map[string]any{
			"name":   "Bob",
			"wallet": "0xbBbBBBBbbBBBbbbBbbBbbbbBBbBbbbbBbBbbBBbB",
		},
		"contents": "Hello, Bob!",
	},
}

func TestParseAndView_etherMail(t *testing.T) {
	from := "0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826"
	addr, typed, raw, err := Parse([]any{from, etherMail})
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(from), addr)
	require.Equal(t, "Mail", typed.PrimaryType)
	require.Equal(t, "Ether Mail", raw["domain"].(map[string]any)["name"])

	fields, err := View(typed)
	require.NoError(t, err)
	require.True(t, containsName(fields, "from"))
	require.True(t, containsName(fields, "contents"))
	require.True(t, containsText(fields, "Hello, Bob!"))
}

func TestParse_jsonString(t *testing.T) {
	b, err := json.Marshal(etherMail)
	require.NoError(t, err)
	_, typed, _, err := Parse([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826", string(b)})
	require.NoError(t, err)
	require.Equal(t, "Mail", typed.PrimaryType)
}

func TestParse_invalid(t *testing.T) {
	_, _, _, err := Parse([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826"})
	require.ErrorIs(t, err, ErrInvalidParams)
	_, _, _, err = Parse([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826", []any{}})
	require.ErrorIs(t, err, ErrInvalidParams)
}

func TestLooksLikeV4(t *testing.T) {
	require.True(t, LooksLikeV4([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826", etherMail}))
	require.False(t, LooksLikeV4([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826", "hello"}))
	require.False(t, LooksLikeV4([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826"}))
}

func TestDisplay_jsonSafeFieldsArray(t *testing.T) {
	from := "0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826"
	_, typed, raw, err := Parse([]any{from, etherMail})
	require.NoError(t, err)

	view, err := Display(typed, raw)
	require.NoError(t, err)
	require.Equal(t, "typedData", view["kind"])
	require.Equal(t, "Mail", view["primaryType"])

	b, err := json.Marshal(view)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(b, &decoded))
	fields, ok := decoded["fields"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, fields)
	require.True(t, containsMapName(fields, "from"))
	require.True(t, containsMapName(fields, "contents"))
}

func TestHash_etherMailStable(t *testing.T) {
	_, typed, _, err := Parse([]any{"0xCD2a3d9F938E13CD947Ec05AbC7FE734Df8DD826", etherMail})
	require.NoError(t, err)
	got, err := Hash(typed)
	require.NoError(t, err)
	require.Equal(t, "0xbe609aee343fb3c4b28e1df9e632fca64fcfaede20f02e86244efddf30957bd2", hexutil.Encode(got))
}

func containsName(fields []Field, name string) bool {
	for _, f := range fields {
		if f.Name == name {
			return true
		}
		if containsName(f.Fields, name) {
			return true
		}
	}
	return false
}

func containsText(fields []Field, text string) bool {
	for _, f := range fields {
		if f.Text == text {
			return true
		}
		if containsText(f.Fields, text) {
			return true
		}
	}
	return false
}

func containsMapName(fields []any, name string) bool {
	for _, item := range fields {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if m["name"] == name {
			return true
		}
		if nested, ok := m["fields"].([]any); ok && containsMapName(nested, name) {
			return true
		}
	}
	return false
}
