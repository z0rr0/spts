// Package auth0 provides authorization methods.

package auth0

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/z0rr0/spts/auth0/token"
)

const (
	// ServerEnv is an environment variable name for server's tokens.
	// It is comma-separated list of "clientID:secret" pairs,
	// where "secret" is a hex-encoded string, but "clientID" is uint16 value from 1 to 16384.
	ServerEnv = "SPTS_TOKENS"

	// ClientEnv is an environment variable name for client's token.
	// Format is "clientID:secret", where "secret" is a hex-encoded string.
	ClientEnv = "SPTS_KEY"
)

var (
	// ErrAuthRequired is an error for required authorization.
	ErrAuthRequired = errors.New("auth0 required")
)

// NewToken returns a new token built from string "clientID:secret".
func NewToken(pair string, chunk uint32) (*token.Token, error) {
	clientPair := strings.Split(pair, ":")
	if n := len(clientPair); n != 2 {
		return nil, errors.Join(token.ErrFormat, fmt.Errorf("invalid pair length: %d", n))
	}

	clientID, err := strconv.ParseUint(clientPair[0], 10, 16)
	if err != nil {
		return nil, errors.Join(token.ErrFormat, fmt.Errorf("clientID: %w", err))
	}

	secret, err := hex.DecodeString(clientPair[1])
	if err != nil {
		return nil, errors.Join(token.ErrFormat, fmt.Errorf("decode hex value: %w", err))
	}

	return &token.Token{ClientID: uint16(clientID), Chunk: chunk, Secret: secret}, nil
}

// ServerTokens loads server's tokens from environment variable.
func ServerTokens(chunk uint32) (map[uint16]*token.Token, error) {
	value := strings.Trim(os.Getenv(ServerEnv), ", ")
	if value == "" {
		return nil, ErrAuthRequired
	}

	pairs := strings.Split(value, ",")
	tokens := make(map[uint16]*token.Token, len(pairs))

	for _, pair := range pairs {
		t, err := NewToken(pair, chunk)
		if err != nil {
			return nil, err
		}

		tokens[t.ClientID] = t
	}

	return tokens, nil
}

// ClientToken returns client's token from environment variable.
func ClientToken() (*token.Token, error) {
	value := strings.Trim(os.Getenv(ClientEnv), " ")

	if value == "" {
		return nil, ErrAuthRequired
	}

	return NewToken(value, 0)
}
