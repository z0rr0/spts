package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	lenAction   = 1
	lenClientID = 2
	lenIP       = 16
	lenSalt     = 32
	lenTime     = 8
	lenSign     = sha512.Size
	lenToken    = lenAction + lenClientID + lenIP + lenSalt + lenTime + lenSign

	endClient = lenAction + lenClientID
	endIP     = endClient + lenIP
	endSalt   = endIP + lenSalt
	endTime   = endSalt + lenTime

	// timestampLimit is a limit for UNIX time difference between client and server.
	timestampLimit = 30 // seconds
)

// Token is a client's token.
type Token struct {
	ClientID  uint16
	Secret    []byte
	Download  bool // false - upload, true - download
	IP        net.IP
	salt      [lenSalt]byte
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
	buf := make([]byte, lenToken)

	if !t.Download {
		buf[0] = 1
	}

	binary.BigEndian.PutUint16(buf[lenAction:], t.ClientID)
	copy(buf[endClient:], t.IP.To16())
	copy(buf[endIP:], t.salt[:])
	binary.BigEndian.PutUint64(buf[endSalt:], uint64(t.timestamp))

	prefixPart := buf[:endTime]

	h := sha512.New()
	h.Write(prefixPart)
	h.Write(t.Secret)

	copy(t.signature[:], h.Sum(nil))
	copy(buf[endTime:], t.signature[:])

	return buf
}

// Verify checks token signature.
func (t *Token) Verify(signature []byte) bool {
	t.Sign() // update signature by current token values
	return hmac.Equal(signature, t.signature[:])
}

// Build resets temporary values and builds new signature.
func (t *Token) Build() ([]byte, error) {
	err := t.init()
	if err != nil {
		return nil, err
	}

	return t.Sign(), nil
}

// Handshake is called by clients to send token to server and receive one back.
func (t *Token) Handshake(rw io.ReadWriter) error {
	if rw == nil {
		return errors.New("nil reader/writer")
	}

	header, err := t.Build()
	if err != nil {
		return err
	}

	// send token to server
	n, err := rw.Write(header)
	if err != nil {
		return fmt.Errorf("failed to write header data: %w", err)
	}

	if n != lenToken {
		return errors.New("invalid write token length")
	}

	// receive reply-token from server
	header = make([]byte, lenToken)
	n, err = rw.Read(header)

	if err != nil {
		return fmt.Errorf("failed to read header data: %w", err)
	}

	if n != lenToken {
		return errors.New("invalid read token length")
	}

	tokens := map[uint16]*Token{t.ClientID: t}
	_, err = verifyHeader(header, tokens)
	return err
}

// Equal checks if two tokens are equal.
// It can be used only for testing, verify signature in production with Verify method.
func (t *Token) Equal(x *Token) bool {
	return t.ClientID == x.ClientID && bytes.Equal(t.Secret, x.Secret)
}

// Action returns token's action.
func (t *Token) Action() string {
	if t.Download {
		return "download"
	}

	return "upload"
}
