package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// AuthorizationHeader is a header name for authorization.
	AuthorizationHeader = "Authorization"

	// Prefix is a prefix for authorization header.
	Prefix = "Bearer "

	// ServerEnv is an environment variable name for server's tokens.
	// It is comma-separated list of "clientID:secret" pairs,
	// where "secret" is a hex-encoded string, but "clientID" is uint16 value.
	ServerEnv = "SPTS_TOKENS"

	// ClientEnv is an environment variable name for client's token.
	// It is a base64 string with format "<clientID><salt><timestamp><signature>", all values are binary.
	// Total size is 106 bytes:
	// 	clientID - uint16, 2 bytes
	// 	salt - client's random value, 32 bytes
	// 	timestamp - int64 UNIX timestamp, 8 bytes (should be synchronized with server with precision 30 seconds)
	// 	signature - SHA512(clientID + salt + timestamp + secret), 64 bytes sha512.Size
	ClientEnv = "SPTS_KEY"

	clientIDLength  = 2  // bytes
	saltLength      = 32 // bytes
	timestampLength = 8  // bytes
	tokenLength     = clientIDLength + saltLength + timestampLength + sha512.Size

	// timestampLimit is a limit for UNIX time difference between client and server.
	timestampLimit = 30 // seconds
)

var (
	// ErrorUnauthorized is an error for unauthorized request.
	ErrorUnauthorized = errors.New("unauthorized")

	// ErrTokenSignature is an error for invalid token signature.
	ErrTokenSignature = errors.New("invalid token signature")

	// ErrTokenFormat is an error for invalid token format.
	ErrTokenFormat = errors.New("invalid token format")
)

// Token is a client's token.
type Token struct {
	ClientID  uint16
	Secret    []byte
	salt      [saltLength]byte
	timestamp int64
	signature [sha512.Size]byte
}

// init sets random salt and current timestamp.
func (t *Token) init() error {
	if t.timestamp == 0 {
		t.timestamp = time.Now().Unix()
	}

	_, err := rand.Read(t.salt[:])
	if err != nil {
		return fmt.Errorf("read random: %w", err)
	}

	return nil
}

// Sign builds token, calculates its signature and returns it with data as common byte slice.
func (t *Token) Sign() []byte {
	const prefixLen = clientIDLength + saltLength + timestampLength
	buf := make([]byte, tokenLength)

	binary.BigEndian.PutUint16(buf, t.ClientID)
	copy(buf[clientIDLength:], t.salt[:])
	binary.BigEndian.PutUint64(buf[clientIDLength+saltLength:], uint64(t.timestamp))

	prefixPart := buf[:prefixLen]

	h := sha512.New()
	h.Write(prefixPart)
	h.Write(t.Secret)

	copy(t.signature[:], h.Sum(nil))
	copy(buf[prefixLen:], t.signature[:])

	return buf
}

// String returns token as base64 string.
func (t *Token) String() string {
	if t.ClientID == 0 {
		return ""
	}

	if err := t.init(); err != nil {
		slog.Error("token_init", "error", err)
		return ""
	}

	return base64.StdEncoding.EncodeToString(t.Sign())
}

// Verify checks token signature.
func (t *Token) Verify(signature []byte) bool {
	t.Sign() // update signature by current token values
	return hmac.Equal(signature, t.signature[:])
}

// Equal checks if two tokens are equal.
// It can be used only for testing, verify signature in production with Verify method.
func (t *Token) Equal(x *Token) bool {
	return t.ClientID == x.ClientID && bytes.Equal(t.Secret, x.Secret)
}

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

// LoadTokens loads server's tokens from environment variable.
func LoadTokens() (map[uint16]*Token, error) {
	value := strings.Trim(os.Getenv(ServerEnv), ", ")
	if value == "" {
		return nil, nil
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
		return &Token{}, nil
	}

	return NewToken(value)
}
