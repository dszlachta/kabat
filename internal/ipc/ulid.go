package ipc

import "github.com/oklog/ulid/v2"

// NewULID returns a 26-character Crockford-base32 ULID.
func NewULID() (string, error) {
	id, err := ulid.New(ulid.Now(), ulid.DefaultEntropy())
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
