// Package auth provides authorization methods.
//
// Authorization token format (bytes):
// +--------+--------+------+------+-----------+-----------+
// | action | client |  IP  | salt | timestamp | signature |
// +--------+--------+------+------+-----------+-----------+
// |    1   |    2   |  16  |  32  |     8     |    64     |
// +--------+--------+------+------+-----------+-----------+

package auth

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// ServerEnv is an environment variable name for server's tokens.
	// It is comma-separated list of "clientID:secret" pairs,
	// where "secret" is a hex-encoded string, but "clientID" is uint16 value.
	ServerEnv = "SPTS_TOKENS"

	// ClientEnv is an environment variable name for client's token.
	ClientEnv = "SPTS_KEY"
)

var (
	// ErrUnauthorized is an error for unauthorized request.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrAuthRequired is an error for required authorization.
	ErrAuthRequired = errors.New("auth required")

	// ErrTokenSignature is an error for invalid token signature.
	ErrTokenSignature = errors.New("invalid token signature")

	// ErrTokenFormat is an error for invalid token format.
	ErrTokenFormat = errors.New("invalid token format")
)

// NewToken returns new token from string "clientID:secret".
func NewToken(pair string) (*Token, error) {
	clientPair := strings.Split(pair, ":")
	if n := len(clientPair); n != 2 {
		return nil, errors.Join(ErrTokenFormat, fmt.Errorf("invalid pair length: %d", n))
	}

	clientID, err := strconv.ParseUint(clientPair[0], 10, 16)
	if err != nil {
		return nil, errors.Join(ErrTokenFormat, fmt.Errorf("clientID: %w", err))
	}

	token, err := hex.DecodeString(clientPair[1])
	if err != nil {
		return nil, errors.Join(ErrTokenFormat, fmt.Errorf("decode hex value: %w", err))
	}

	return &Token{ClientID: uint16(clientID), Secret: token}, nil
}

// ServerTokens loads server's tokens from environment variable.
func ServerTokens() (map[uint16]*Token, error) {
	value := strings.Trim(os.Getenv(ServerEnv), ", ")
	if value == "" {
		return nil, ErrAuthRequired
	}

	pairs := strings.Split(value, ",")
	tokens := make(map[uint16]*Token, len(pairs))

	for _, pair := range pairs {
		token, err := NewToken(pair)
		if err != nil {
			return nil, err
		}

		tokens[token.ClientID] = token
	}

	return tokens, nil
}

// ClientToken returns client's token from environment variable.
func ClientToken() (*Token, error) {
	value := strings.Trim(os.Getenv(ClientEnv), " ")

	if value == "" {
		return nil, ErrAuthRequired
	}

	return NewToken(value)
}

// Verify is called by servers, it checks authorization header and returns new token if `r` data is valid.
func Verify(r io.Reader, tokens map[uint16]*Token) (*Token, error) {
	if r == nil {
		return nil, errors.Join(ErrUnauthorized, errors.New("nil reader"))
	}

	header := make([]byte, lenToken)
	n, err := r.Read(header)

	if err != nil {
		return nil, errors.Join(ErrUnauthorized, fmt.Errorf("failed to read header data: %w", err))
	}

	if n != lenToken {
		return nil, errors.Join(ErrUnauthorized, errors.New("invalid token length"))
	}

	return verifyHeader(header, tokens)
}

func verifyHeader(header []byte, serverTokens map[uint16]*Token) (*Token, error) {
	var clientID uint16

	if n := len(header); n != lenToken {
		return nil, fmt.Errorf("invalid header length: %d", n)
	}

	clientIDBytes := header[lenAction:endClient]
	err := binary.Read(bytes.NewReader(clientIDBytes), binary.BigEndian, &clientID)
	if err != nil {
		return nil, errors.Join(ErrUnauthorized, fmt.Errorf("clientID parse: %w", err))
	}

	serverToken, ok := serverTokens[clientID]
	if !ok {
		return nil, errors.Join(ErrUnauthorized, fmt.Errorf("unknown clientID: %d", clientID))
	}

	timestamp, err := verifyTimestamp(header[endSalt:endTime])
	if err != nil {
		return nil, err
	}

	token := &Token{
		ClientID:  clientID,
		Secret:    serverToken.Secret,
		Download:  header[0] == 0,
		IP:        net.IP(header[endClient:endIP]),
		timestamp: timestamp,
	}

	copy(token.salt[:], header[endIP:endSalt])
	signature := header[endTime:]

	if !token.Verify(signature) {
		return nil, ErrTokenSignature
	}

	// token is correct, reinitialize it to reset timestamp and salt for response
	if err = token.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize token: %w", err)
	}

	return token, nil
}

func verifyTimestamp(value []byte) (int64, error) {
	var timestamp int64

	err := binary.Read(bytes.NewReader(value), binary.BigEndian, &timestamp)
	if err != nil {
		return 0, errors.Join(ErrUnauthorized, fmt.Errorf("timestamp parse: %w", err))
	}

	timeDiff := time.Now().Unix() - timestamp
	if timeDiff > timestampLimit || timeDiff < -timestampLimit {
		return 0, errors.Join(
			ErrUnauthorized,
			fmt.Errorf("not synchronized time, diff=%d, but abs limit=%d", timeDiff, timestampLimit),
		)
	}

	return timestamp, nil
}
