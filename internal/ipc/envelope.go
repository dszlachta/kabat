package ipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const maxFrame = 1024 * 1024

var (
	ErrMissingULID   = errors.New("missing ulid")
	ErrMissingMethod = errors.New("missing method")
	ErrFrameTooLarge = errors.New("frame too large")
)

// Envelope wraps an EIP-1193 payload for the Unix socket / native-messaging link.
type Envelope struct {
	ULID    string          `json:"ulid"`
	TS      int64           `json:"ts"`
	Payload json.RawMessage `json:"payload"`
}

// RequestPayload is a standard EIP-1193 request.
type RequestPayload struct {
	ULID        string `json:"-"`
	Method      string `json:"method"`
	Params      any    `json:"params"`
	Origin      string `json:"origin,omitempty"`
	SessionULID string `json:"sessionUlid,omitempty"`
}

// ResultPayload is a standard EIP-1193 result, error, or event.
type ResultPayload struct {
	Result any       `json:"result,omitempty"`
	Error  *RPCError `json:"error,omitempty"`
	Event  string    `json:"event,omitempty"`
	Params any       `json:"params,omitempty"`
}

// RPCError is the EIP-1193 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// NowMS returns Unix time in milliseconds.
func NowMS() int64 {
	return time.Now().UnixMilli()
}

// DecodeEnvelope unmarshals an envelope and requires a ulid.
func DecodeEnvelope(data []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, err
	}
	if env.ULID == "" {
		return Envelope{}, ErrMissingULID
	}
	return env, nil
}

// ParseRequest reads method/params from an envelope payload.
func ParseRequest(env Envelope) (RequestPayload, error) {
	var req RequestPayload
	if len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return RequestPayload{}, err
		}
	}
	if req.Method == "" {
		return RequestPayload{}, ErrMissingMethod
	}
	return req, nil
}

// Reply builds a result envelope that echoes ulid.
func Reply(ulid string, payload ResultPayload) (Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{ULID: ulid, TS: NowMS(), Payload: b}, nil
}

// Event builds an unsolicited event envelope with a new ulid.
func Event(name string, params any) (Envelope, error) {
	b, err := json.Marshal(ResultPayload{Event: name, Params: params})
	if err != nil {
		return Envelope{}, err
	}
	id, err := NewULID()
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{ULID: id, TS: NowMS(), Payload: b}, nil
}

// WriteFrame writes a native-messaging length-prefixed JSON frame.
func WriteFrame(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(body) > maxFrame {
		return ErrFrameTooLarge
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(body)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(body)
	return err
}

// ReadFrame reads a native-messaging length-prefixed JSON frame.
func ReadFrame(r io.Reader, v any) error {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n > maxFrame {
		return ErrFrameTooLarge
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// ReadEnvelope reads one framed envelope.
func ReadEnvelope(r io.Reader) (Envelope, error) {
	var raw json.RawMessage
	if err := ReadFrame(r, &raw); err != nil {
		return Envelope{}, err
	}
	return DecodeEnvelope(raw)
}

// EncodeFrame returns the framed bytes for v (for tests).
func EncodeFrame(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
