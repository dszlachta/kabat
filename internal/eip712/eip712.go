package eip712

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

var ErrInvalidParams = errors.New("invalid typed data params")

// Field is a UI node for the confirm dialog (no EIP-712 rules).
type Field struct {
	Name   string  `json:"name"`
	Type   string  `json:"type,omitempty"`
	Text   string  `json:"text,omitempty"`
	Fields []Field `json:"fields,omitempty"`
}

// LooksLikeV4 reports whether params are [address, v4 typed data].
func LooksLikeV4(params any) bool {
	list := asList(params)
	if len(list) < 2 {
		return false
	}
	m := asObject(list[1])
	if m == nil {
		return false
	}
	_, types := m["types"]
	_, primary := m["primaryType"]
	_, domain := m["domain"]
	_, message := m["message"]
	return types && primary && domain && message
}

// Parse reads [address, data] into an address, typed data, and original JSON object.
func Parse(params any) (common.Address, apitypes.TypedData, map[string]any, error) {
	list := asList(params)
	if len(list) < 2 {
		return common.Address{}, apitypes.TypedData{}, nil, ErrInvalidParams
	}
	from, err := parseAddress(list[0])
	if err != nil {
		return common.Address{}, apitypes.TypedData{}, nil, err
	}
	raw := asObject(list[1])
	if raw == nil {
		return common.Address{}, apitypes.TypedData{}, nil, ErrInvalidParams
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return common.Address{}, apitypes.TypedData{}, nil, ErrInvalidParams
	}
	var typed apitypes.TypedData
	if err := json.Unmarshal(b, &typed); err != nil {
		return common.Address{}, apitypes.TypedData{}, nil, ErrInvalidParams
	}
	if typed.PrimaryType == "" || typed.Types == nil || typed.Message == nil {
		return common.Address{}, apitypes.TypedData{}, nil, ErrInvalidParams
	}
	return from, typed, raw, nil
}

// View is a nested name/value tree from TypedData.Format.
func View(typed apitypes.TypedData) ([]Field, error) {
	nvts, err := typed.Format()
	if err != nil {
		return nil, err
	}
	return fieldsFromNVTs(nvts), nil
}

// Display is the confirm-dialog payload: JSON-safe maps only (Wails events).
func Display(typed apitypes.TypedData, raw map[string]any) (map[string]any, error) {
	fields, err := View(typed)
	if err != nil {
		return nil, err
	}
	view := map[string]any{
		"kind":        "typedData",
		"title":       "Sign typed data",
		"primaryType": typed.PrimaryType,
		"fields":      fields,
		"raw":         raw,
	}
	b, err := json.Marshal(view)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Hash is the EIP-712 digest to sign.
func Hash(typed apitypes.TypedData) ([]byte, error) {
	digest, _, err := apitypes.TypedDataAndHash(typed)
	return digest, err
}

func fieldsFromNVTs(nvts []*apitypes.NameValueType) []Field {
	out := make([]Field, 0, len(nvts))
	for _, nvt := range nvts {
		if nvt == nil {
			continue
		}
		f := Field{Name: nvt.Name, Type: nvt.Typ}
		if nested := nestedNVTs(nvt.Value); nested != nil {
			f.Fields = fieldsFromNVTs(nested)
		} else if nvt.Value != nil {
			f.Text = fmt.Sprint(nvt.Value)
		}
		out = append(out, f)
	}
	return out
}

func nestedNVTs(v any) []*apitypes.NameValueType {
	switch n := v.(type) {
	case []*apitypes.NameValueType:
		return n
	case []apitypes.NameValueType:
		out := make([]*apitypes.NameValueType, len(n))
		for i := range n {
			out[i] = &n[i]
		}
		return out
	default:
		return nil
	}
}

func asList(params any) []any {
	if params == nil {
		return nil
	}
	switch p := params.(type) {
	case []any:
		return p
	default:
		return []any{p}
	}
}

func asObject(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	s, ok := v.(string)
	if !ok {
		b, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		return m
	}
	var m map[string]any
	if json.Unmarshal([]byte(s), &m) != nil {
		return nil
	}
	return m
}

func parseAddress(v any) (common.Address, error) {
	switch a := v.(type) {
	case string:
		if !common.IsHexAddress(a) {
			return common.Address{}, ErrInvalidParams
		}
		return common.HexToAddress(a), nil
	case common.Address:
		return a, nil
	default:
		return common.Address{}, ErrInvalidParams
	}
}
