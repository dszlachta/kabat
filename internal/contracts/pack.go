package contracts

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Pack encodes selector + args using the contract ABI.
func (c *Contract) Pack(selector string, args []string) ([]byte, abi.Method, error) {
	if c == nil {
		return nil, abi.Method{}, ErrNotContract
	}
	id, err := decodeSelector(selector)
	if err != nil {
		return nil, abi.Method{}, err
	}

	mu.RLock()
	method, err := c.ABI.MethodById(id)
	if err != nil {
		mu.RUnlock()
		return nil, abi.Method{}, err
	}
	copied := copyMethod(method)
	mu.RUnlock()

	if args == nil {
		args = []string{}
	}
	if len(args) != len(copied.Inputs) {
		return nil, copied, fmt.Errorf("expected %d argument(s), got %d", len(copied.Inputs), len(args))
	}

	values := make([]any, len(args))
	for i, raw := range args {
		v, err := parseArg(copied.Inputs[i], raw)
		if err != nil {
			return nil, copied, err
		}
		values[i] = v
	}
	packed, err := copied.Inputs.Pack(values...)
	if err != nil {
		return nil, copied, err
	}
	return append(append([]byte(nil), copied.ID...), packed...), copied, nil
}

func copyMethod(m *abi.Method) abi.Method {
	copied := *m
	copied.ID = append([]byte(nil), m.ID...)
	copied.Inputs = append(abi.Arguments(nil), m.Inputs...)
	copied.Outputs = append(abi.Arguments(nil), m.Outputs...)
	return copied
}

func decodeSelector(selector string) ([]byte, error) {
	b, err := hexutil.Decode(strings.TrimSpace(selector))
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		return nil, errors.New("invalid selector")
	}
	return b[:4], nil
}

func parseArg(arg abi.Argument, s string) (any, error) {
	label := strings.TrimSpace(arg.Type.String() + " " + arg.Name)
	v, err := parseValue(arg.Type, s)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return v, nil
}

func parseValue(typ abi.Type, s string) (any, error) {
	switch typ.T {
	case abi.StringTy:
		return s, nil
	case abi.UintTy:
		return parseUint(s, typ.Size)
	case abi.IntTy:
		return parseInt(s, typ.Size)
	case abi.AddressTy:
		return parseAddress(s)
	case abi.BoolTy:
		return parseBool(s)
	case abi.BytesTy:
		return decodeHex(s)
	case abi.FixedBytesTy:
		b, err := decodeHex(s)
		if err != nil {
			return nil, err
		}
		if len(b) == 32 && typ.Size < 32 {
			b = b[:typ.Size]
		}
		if len(b) != typ.Size {
			return nil, fmt.Errorf("expected %d bytes", typ.Size)
		}
		return b, nil
	case abi.SliceTy, abi.ArrayTy, abi.TupleTy:
		return parseABIEncoded(typ, s)
	default:
		return parseABIEncoded(typ, s)
	}
}

func parseUint(s string, bits int) (*big.Int, error) {
	n, err := parseBig(s, false)
	if err != nil {
		return nil, err
	}
	if n.Sign() < 0 || n.BitLen() > bits {
		return nil, fmt.Errorf("overflow uint%d", bits)
	}
	return n, nil
}

func parseInt(s string, bits int) (*big.Int, error) {
	n, err := parseBig(s, true)
	if err != nil {
		return nil, err
	}
	max := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
	min := new(big.Int).Neg(max)
	max.Sub(max, big.NewInt(1))
	if n.Cmp(min) < 0 || n.Cmp(max) > 0 {
		return nil, fmt.Errorf("overflow int%d", bits)
	}
	return n, nil
}

func parseBig(s string, signed bool) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("invalid number")
	}
	if hasHexPrefix(s) {
		b, err := decodeHex(s)
		if err != nil {
			return nil, errors.New("invalid number")
		}
		n := new(big.Int).SetBytes(b)
		if signed && len(b) == 32 && b[0]&0x80 != 0 {
			n.Sub(n, new(big.Int).Lsh(big.NewInt(1), 256))
		}
		return n, nil
	}
	if !signed && s[0] == '-' {
		return nil, errors.New("invalid number")
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, errors.New("invalid number")
	}
	return n, nil
}

func parseAddress(s string) (common.Address, error) {
	b, err := decodeHex(s)
	if err != nil {
		return common.Address{}, err
	}
	if len(b) != 20 && len(b) != 32 {
		return common.Address{}, errors.New("invalid address")
	}
	return common.BytesToAddress(b), nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	}
	if hasHexPrefix(s) {
		b, err := decodeHex(s)
		if err != nil {
			return false, err
		}
		for _, x := range b {
			if x != 0 {
				return true, nil
			}
		}
		return false, nil
	}
	return false, errors.New("invalid bool")
}

func parseABIEncoded(typ abi.Type, s string) (any, error) {
	b, err := decodeHex(s)
	if err != nil {
		return nil, err
	}
	unpacked, err := abi.Arguments{{Type: typ}}.Unpack(b)
	if err != nil {
		return nil, err
	}
	if len(unpacked) != 1 {
		return nil, errors.New("invalid abi encoding")
	}
	return unpacked[0], nil
}

func hasHexPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X')
}

func decodeHex(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if !hasHexPrefix(s) {
		return nil, hexutil.ErrMissingPrefix
	}
	digits := s[2:]
	if digits == "" {
		return []byte{}, nil
	}
	if len(digits)%2 == 1 {
		digits = "0" + digits
	}
	b, err := hex.DecodeString(digits)
	if err != nil {
		return nil, err
	}
	return b, nil
}
