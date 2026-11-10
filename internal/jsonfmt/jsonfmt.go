package jsonfmt

import (
	"bytes"
	"encoding/json"
)

// Bytes pretty-prints src with json.Indent (two-space indent).
// Invalid JSON is returned unchanged.
func Bytes(src []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, src, "", "  "); err != nil {
		return string(src)
	}
	return buf.String()
}

// Value marshals v to JSON, then pretty-prints it with json.Indent.
func Value(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return Bytes(b)
}
