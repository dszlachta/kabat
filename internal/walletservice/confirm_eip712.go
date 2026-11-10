package walletservice

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/dszlachta/kabat/internal/chain"
	"github.com/dszlachta/kabat/internal/contracts"
	"github.com/dszlachta/kabat/internal/eip712"
	"github.com/dszlachta/kabat/internal/jsonfmt"
	"github.com/dszlachta/kabat/internal/provider"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
)

func (s *WalletService) confirmPayload(ctx context.Context, tx provider.TransactionRequest) map[string]any {
	view := jsonMap(tx.Raw)
	if kind, _ := view["kind"].(string); kind != "typedData" {
		s.attachContractEIP712(ctx, view, tx)
		view = jsonMap(view)
	}
	printed := any(view)
	if kind, _ := view["kind"].(string); kind == "typedData" {
		if raw := view["raw"]; raw != nil {
			printed = raw
		}
	}
	if pretty := jsonfmt.Value(printed); pretty != "" {
		view["rawJSON"] = pretty
	}
	return view
}

func (s *WalletService) attachContractEIP712(ctx context.Context, view map[string]any, tx provider.TransactionRequest) {
	if tx.To == nil {
		return
	}
	s.ensureStoredABI(*tx.To)
	c, ok := contracts.Get(*tx.To)
	if !ok || c == nil || c.EIP712 == nil || len(c.EIP712.Types) == 0 {
		return
	}
	domainVals, msgVals := s.eip712Values(ctx, c, tx)
	fields := eip712SchemaFields(c.EIP712, domainVals, msgVals)
	if len(fields) == 0 {
		return
	}
	view["fields"] = fields
	if c.EIP712.PrimaryType != "" {
		view["primaryType"] = c.EIP712.PrimaryType
	}
}

func (s *WalletService) eip712Values(ctx context.Context, c *contracts.Contract, tx provider.TransactionRequest) (map[string]string, map[string]string) {
	domainVals := map[string]string{}
	if tx.To != nil {
		domainVals["verifyingContract"] = tx.To.Hex()
	}
	msgVals := callArgValues(c, tx.Data)

	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.mu.Lock()
	url := s.url
	s.mu.Unlock()
	client, err := chain.ConnectURL(callCtx, url)
	if err != nil {
		return domainVals, msgVals
	}
	defer client.Close()
	if id, err := client.ChainID(callCtx); err == nil && id != nil {
		domainVals["chainId"] = id.String()
	}

	ethCall := func(data []byte) ([]byte, error) {
		return client.Call(callCtx, ethereum.CallMsg{
			From: tx.From,
			To:   tx.To,
			Data: data,
		})
	}
	mergeVals(domainVals, callEIP712Domain(c, ethCall))
	mergeVals(msgVals, callEIP712Message(c, tx.Data, ethCall))
	return domainVals, msgVals
}

func callArgValues(c *contracts.Contract, data []byte) map[string]string {
	if c == nil || len(data) < 4 {
		return map[string]string{}
	}
	method, err := c.ABI.MethodById(data[:4])
	if err != nil || method == nil {
		return map[string]string{}
	}
	args, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(method.Inputs))
	for i, arg := range method.Inputs {
		if i >= len(args) {
			continue
		}
		if arg.Name != "" {
			out[arg.Name] = formatValue(args[i])
		}
	}
	return out
}

func mergeVals(dst, src map[string]string) {
	if dst == nil || src == nil {
		return
	}
	for k, v := range src {
		dst[k] = v
	}
}

func canonName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '_' {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func lookupVal(vals map[string]string, name string) (string, bool) {
	if vals == nil || name == "" {
		return "", false
	}
	if v, ok := vals[name]; ok {
		return v, true
	}
	want := canonName(name)
	for k, v := range vals {
		if canonName(k) == want {
			return v, true
		}
	}
	return "", false
}

func eip712SchemaFields(schema *contracts.EIP712, domainVals, msgVals map[string]string) []eip712.Field {
	if schema == nil {
		return nil
	}
	out := make([]eip712.Field, 0, len(schema.Types))
	for _, name := range eip712TypeOrder(schema) {
		vals := msgVals
		if name == "EIP712Domain" {
			vals = domainVals
		} else if name != schema.PrimaryType {
			vals = nil
		}
		out = append(out, eip712TypeField(name, schema.Types[name], vals))
	}
	return out
}

func eip712TypeOrder(schema *contracts.EIP712) []string {
	var rest []string
	hasDomain := false
	hasPrimary := false
	for name := range schema.Types {
		switch name {
		case "EIP712Domain":
			hasDomain = true
		case schema.PrimaryType:
			hasPrimary = true
		default:
			rest = append(rest, name)
		}
	}
	slices.Sort(rest)
	names := make([]string, 0, len(schema.Types))
	if hasDomain {
		names = append(names, "EIP712Domain")
	}
	if hasPrimary && schema.PrimaryType != "" {
		names = append(names, schema.PrimaryType)
	}
	return append(names, rest...)
}

func eip712TypeField(name string, spec []contracts.EIP712Field, vals map[string]string) eip712.Field {
	f := eip712.Field{Name: name, Type: name}
	f.Fields = make([]eip712.Field, 0, len(spec))
	for _, member := range spec {
		child := eip712.Field{Name: member.Name, Type: member.Type}
		if text, ok := lookupVal(vals, member.Name); ok {
			child.Text = text
		}
		f.Fields = append(f.Fields, child)
	}
	return f
}

func callEIP712Domain(c *contracts.Contract, call func([]byte) ([]byte, error)) map[string]string {
	method, ok := c.ABI.Methods["eip712Domain"]
	if !ok {
		return nil
	}
	out, err := call(method.ID)
	if err != nil {
		return nil
	}
	vals, err := method.Outputs.Unpack(out)
	if err != nil || len(vals) < 5 {
		return nil
	}
	names := []string{"", "name", "version", "chainId", "verifyingContract"}
	mapped := make(map[string]string, 4)
	for i := 1; i < len(names) && i < len(vals); i++ {
		mapped[names[i]] = formatValue(vals[i])
	}
	return mapped
}

func callEIP712Message(c *contracts.Contract, data []byte, call func([]byte) ([]byte, error)) map[string]string {
	if c.EIP712 == nil || c.EIP712.PrimaryType == "" || len(data) < 4 {
		return nil
	}
	getter, ok := c.ABI.Methods["eip712_"+c.EIP712.PrimaryType]
	if !ok || len(getter.Outputs) != 1 {
		return nil
	}
	method, err := c.ABI.MethodById(data[:4])
	if err != nil || method == nil {
		return nil
	}
	args, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		return nil
	}
	bound, ok := bindArgsByName(getter.Inputs, method.Inputs, args)
	if !ok {
		return nil
	}
	packed, err := getter.Inputs.Pack(bound...)
	if err != nil {
		return nil
	}
	out, err := call(append(append([]byte(nil), getter.ID...), packed...))
	if err != nil {
		return nil
	}
	return unpackNamed(getter, out)
}

func bindArgsByName(want, have abi.Arguments, vals []any) ([]any, bool) {
	if len(want) == 0 {
		return []any{}, true
	}
	byName := map[string]any{}
	for i, arg := range have {
		if arg.Name == "" || i >= len(vals) {
			continue
		}
		byName[canonName(arg.Name)] = vals[i]
	}
	out := make([]any, len(want))
	for i, arg := range want {
		v, ok := byName[canonName(arg.Name)]
		if !ok {
			return nil, false
		}
		out[i] = v
	}
	return out, true
}

func unpackNamed(method abi.Method, data []byte) map[string]string {
	vals, err := method.Outputs.Unpack(data)
	if err != nil || len(vals) == 0 {
		return nil
	}
	if len(method.Outputs) == 1 && method.Outputs[0].Type.T == abi.TupleTy {
		comps := method.Outputs[0].Type.TupleRawNames
		nested := anyList(vals[0])
		if nested == nil {
			nested = vals
		}
		return zipNames(comps, nested)
	}
	out := make(map[string]string, len(method.Outputs))
	for i, arg := range method.Outputs {
		if arg.Name == "" || i >= len(vals) {
			continue
		}
		out[arg.Name] = formatValue(vals[i])
	}
	return out
}

func zipNames(names []string, vals []any) map[string]string {
	n := min(len(names), len(vals))
	out := make(map[string]string, n)
	for i := 0; i < n; i++ {
		if names[i] == "" {
			continue
		}
		out[names[i]] = formatValue(vals[i])
	}
	return out
}

func anyList(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return nil
		}
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			out := make([]any, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				out[i] = rv.Index(i).Interface()
			}
			return out
		case reflect.Struct:
			out := make([]any, rv.NumField())
			for i := 0; i < rv.NumField(); i++ {
				out[i] = rv.Field(i).Interface()
			}
			return out
		}
		return nil
	}
}

func jsonMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		b, err := json.Marshal(m)
		if err != nil {
			return maps.Clone(m)
		}
		var out map[string]any
		if json.Unmarshal(b, &out) != nil || out == nil {
			return maps.Clone(m)
		}
		return out
	}
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil || out == nil {
		return map[string]any{}
	}
	return out
}
