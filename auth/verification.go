package auth

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Authorize checks authorization header and returns true if it is valid.
func Authorize(r *http.Request, serverTokens map[uint16]*Token) error {
	if len(serverTokens) == 0 {
		return nil // no tokens, authorization is not required
	}

	token, err := parseHeader(r.Header.Get(AuthorizationHeader))
	if err != nil {
		return err
	}

	return verifyToken(token, serverTokens)
}

func parseHeader(header string) ([]byte, error) {
	if header == "" {
		return nil, errors.New("authorization header is empty")
	}

	if !strings.HasPrefix(header, Prefix) {
		return nil, errors.Join(ErrorUnauthorized, errors.New("invalid authorization header prefix"))
	}

	token, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, Prefix))
	if err != nil {
		return nil, errors.Join(ErrorUnauthorized, fmt.Errorf("base64 decode: %w", err))
	}

	if len(token) != tokenLength {
		return nil, errors.Join(ErrorUnauthorized, fmt.Errorf("invalid token length: %d", len(token)))
	}

	return token, nil
}

func verifyToken(token []byte, serverTokens map[uint16]*Token) error {
	const (
		clientIDLength  = 2
		timestampLength = 8
	)
	var clientID uint16

	clientIDBytes := token[:clientIDLength]
	err := binary.Read(bytes.NewReader(clientIDBytes), binary.BigEndian, &clientID)

	if err != nil {
		return errors.Join(ErrorUnauthorized, fmt.Errorf("clientID parse: %w", err))
	}

	serverToken, ok := serverTokens[clientID]
	if !ok {
		return errors.Join(ErrorUnauthorized, fmt.Errorf("unknown clientID: %d", clientID))
	}

	offset := clientIDLength + saltLength
	timestamp, err := verifyTimestamp(token[offset : offset+timestampLength])

	if err != nil {
		return err
	}

	clientToken := &Token{
		ClientID:  clientID,
		Secret:    serverToken.Secret,
		timestamp: timestamp,
	}

	copy(clientToken.Salt[:], token[clientIDLength:offset])
	signature := token[offset+timestampLength:]

	return clientToken.Verify(signature)
}

func verifyTimestamp(value []byte) (int64, error) {
	var timestamp int64

	err := binary.Read(bytes.NewReader(value), binary.BigEndian, &timestamp)
	if err != nil {
		return 0, errors.Join(ErrorUnauthorized, fmt.Errorf("timestamp parse: %w", err))
	}

	timeDiff := time.Now().Unix() - timestamp
	if timeDiff > timestampLimit || timeDiff < -timestampLimit {
		return 0, errors.Join(
			ErrorUnauthorized,
			fmt.Errorf("not synchronized time, diff=%d, but abs limit=%d", timeDiff, timestampLimit),
		)
	}

	return timestamp, nil
}
