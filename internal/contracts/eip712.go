package contracts

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const eip712MethodPrefix = "eip712_"

var eip712DomainFields = []EIP712Field{
	{Name: "name", Type: "string"},
	{Name: "version", Type: "string"},
	{Name: "chainId", Type: "uint256"},
	{Name: "verifyingContract", Type: "address"},
}

// EIP712Field is one typed-data member (name + solidity type).
type EIP712Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EIP712 is the typed-data schema for a contract (domain + primary types).
type EIP712 struct {
	PrimaryType string                   `json:"primaryType,omitempty"`
	Types       map[string][]EIP712Field `json:"types,omitempty"`
}

func parseEIP712Value(v any) *EIP712 {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out EIP712
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	if out.PrimaryType == "" && len(out.Types) == 0 {
		return nil
	}
	return &out
}

// DeriveEIP712 builds a schema from EIP-5267 `eip712Domain` and `eip712_<Type>` ABI methods.
func DeriveEIP712(spec abi.ABI) *EIP712 {
	types := map[string][]EIP712Field{}
	if _, ok := spec.Methods["eip712Domain"]; ok {
		types["EIP712Domain"] = slices.Clone(eip712DomainFields)
	}
	var names []string
	for _, m := range spec.Methods {
		name, ok := strings.CutPrefix(m.RawName, eip712MethodPrefix)
		if !ok || name == "" {
			continue
		}
		if len(m.Outputs) != 1 || m.Outputs[0].Type.T != abi.TupleTy {
			continue
		}
		types[name] = tupleFields(m.Outputs[0].Type)
		names = append(names, name)
	}
	if len(types) == 0 {
		return nil
	}
	slices.Sort(names)
	primary := ""
	if len(names) > 0 {
		primary = names[0]
	}
	return &EIP712{PrimaryType: primary, Types: types}
}

func tupleFields(t abi.Type) []EIP712Field {
	out := make([]EIP712Field, len(t.TupleElems))
	for i, elem := range t.TupleElems {
		if elem == nil {
			continue
		}
		name := ""
		if i < len(t.TupleRawNames) {
			name = t.TupleRawNames[i]
		}
		out[i] = EIP712Field{Name: name, Type: elem.String()}
	}
	return out
}
