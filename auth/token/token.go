// Package token contains authorization token implementation.
//
// Authorization token format (74 bytes):
// +--------+---------+----------+-----------+---------+----------+-----------+
// | action |  client |    IP    | timestamp |  chunk  |   salt   | signature |
// +--------+---------+----------+-----------+---------+----------+-----------+
// | 1 bit  | 15 bits | 16 bytes |  8 bytes  | 4 bytes | 12 bytes | 32 bytes  |
// +--------+---------+----------+-----------+---------+----------+-----------+
//
// action    - 1 is download, 0 is upload
// client    - client ID. It supports 2^15 = 32768 different values.
// IP        - client's IP address, 16 bytes for IPv6
// salt      - random salt, it has 2^96 possible values.
// timestamp - UNIX time.
// chunk     - it's a size of data chunk in kilobytes. A value 0 is stop flag. Max (2^32 - 1) KB = 4 TB.
//
// signature - SHA-512/256 message hash sum.
package token

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
	lenClientID = 2 // BigEndian, senior bit=1 is a flag for download, least 15 bits is a client ID
	lenIP       = 16
	lenSalt     = 12
	lenTime     = 8
	lenChunk    = 4
	lenSign     = sha512.Size256

	endClient  = lenClientID
	endIP      = endClient + lenIP
	endTime    = endIP + lenTime
	endChunk   = endTime + lenChunk
	endPayload = endChunk + lenSalt

	// timestampLimit is a limit for UNIX time difference between client and server.
	timestampLimit        = 30      // seconds
	actionBit      uint16 = 1 << 15 // download / upload flag

	// string representation of actions
	actionDownload = "download"
	actionUpload   = "upload"

	// Size is a token size in bytes.
	Size = lenClientID + lenIP + lenSalt + lenTime + lenChunk + lenSign

	// ChunkSize is a size of data chunk in kilobytes.
	ChunkSize uint32 = 64
	// MaxChunkSize is a maximum size of data chunk in kilobytes.
	MaxChunkSize uint32 = 4294967295 // 2^32 - 1
)

var (
	// ErrSignature is an error for invalid token signature.
	ErrSignature = errors.New("invalid token signature")

	// ErrFormat is an error for invalid token format.
	ErrFormat = errors.New("invalid token format")
)

// Token is a client's token.
type Token struct {
	Download  bool   // true - download, false - upload
	Secret    []byte // not a part of transmitted data
	ClientID  uint16
	Chunk     uint32
	IP        net.IP
	timestamp int64
	salt      [lenSalt]byte
	signature [lenSign]byte
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

// Sign parses token's fields, calculates its signature and returns it as a byte slice for sending.
func (t *Token) Sign() ([]byte, error) {
	var prefix = t.ClientID

	// the 1st bit is always zero, they can be set only from token fields
	if t.ClientID&actionBit != 0 {
		return nil, errors.Join(ErrSignature, errors.New("invalid clientID"))
	}

	buf := make([]byte, Size)
	if t.Download {
		prefix = prefix | actionBit // set 2nd bit
	}

	binary.BigEndian.PutUint16(buf[:], prefix)
	copy(buf[endClient:], t.IP.To16())
	binary.BigEndian.PutUint64(buf[endIP:], uint64(t.timestamp))
	binary.BigEndian.PutUint32(buf[endTime:], t.Chunk)
	copy(buf[endChunk:], t.salt[:])

	payload := buf[:endPayload]

	h := sha512.New512_256()
	if _, err := h.Write(payload); err != nil {
		return nil, errors.Join(ErrSignature, fmt.Errorf("failed to write prefix: %w", err))
	}
	if _, err := h.Write(t.Secret); err != nil {
		return nil, errors.Join(ErrSignature, fmt.Errorf("failed to write secret: %w", err))
	}

	// update signature and write it to the end of the buffer
	copy(t.signature[:], h.Sum(nil))
	copy(buf[endPayload:], t.signature[:])

	return buf, nil
}

// Verify checks token signature.
func (t *Token) Verify(signature []byte) error {
	// update signature by current token values
	if _, err := t.Sign(); err != nil {
		return err
	}

	if !hmac.Equal(signature, t.signature[:]) {
		return ErrSignature
	}

	return nil
}

// Build resets temporary values and builds new signature.
func (t *Token) Build() ([]byte, error) {
	if err := t.init(); err != nil {
		return nil, err
	}

	return t.Sign()
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

	if n != Size {
		return errors.New("invalid write token length")
	}

	// receive reply-token from server
	header = make([]byte, Size)
	if n, err = rw.Read(header); err != nil {
		return fmt.Errorf("failed to read header data: %w", err)
	}

	if n != Size {
		return errors.New("invalid read token length")
	}

	tokens := map[uint16]*Token{t.ClientID: t}
	_, err = Parse(header, tokens)
	return err
}

// Equal checks if two tokens are equal.
// It can be used only for testing, verify signature in production with Parse method.
func (t *Token) Equal(x *Token) bool {
	return t.ClientID == x.ClientID && bytes.Equal(t.Secret, x.Secret)
}

// Action returns token's action.
func (t *Token) Action() string {
	if t.Download {
		return actionDownload
	}

	return actionUpload
}

// Stop returns true if token's Chunk is 0.
func (t *Token) Stop() bool {
	return t.Chunk == 0
}

// Parse builds token from byte slice and checks its signature.
func Parse(value []byte, serverTokens map[uint16]*Token) (*Token, error) {
	var clientID uint16

	if n := len(value); n != Size {
		return nil, errors.Join(ErrFormat, fmt.Errorf("invalid value length: %d", n))
	}

	clientIDBytes := value[:endClient]
	err := binary.Read(bytes.NewReader(clientIDBytes), binary.BigEndian, &clientID)
	if err != nil {
		return nil, errors.Join(ErrFormat, fmt.Errorf("clientID parse: %w", err))
	}

	download := (clientID & actionBit) != 0
	clientID &= ^actionBit // clear 1st bit

	serverToken, ok := serverTokens[clientID]
	if !ok {
		return nil, errors.Join(ErrFormat, fmt.Errorf("unknown clientID: %d", clientID))
	}

	timestamp, err := verifyTimestamp(value[endIP:endTime])
	if err != nil {
		return nil, err
	}

	chunk, err := verifyChunk(value[endTime:endChunk])
	if err != nil {
		return nil, err
	}

	token := &Token{
		Download:  download,
		ClientID:  clientID,
		Secret:    serverToken.Secret,
		IP:        net.IP(value[endClient:endIP]),
		timestamp: timestamp,
		Chunk:     chunk,
	}

	copy(token.salt[:], value[endChunk:endPayload])
	signature := value[endPayload:]

	if err = token.Verify(signature); err != nil {
		return nil, err
	}

	// token is correct, reinitialize it to reset timestamp and salt for response
	if err = token.init(); err != nil {
		return nil, errors.Join(ErrFormat, fmt.Errorf("failed to initialize token: %w", err))
	}

	return token, nil
}

func verifyTimestamp(value []byte) (int64, error) {
	var timestamp int64

	err := binary.Read(bytes.NewReader(value), binary.BigEndian, &timestamp)
	if err != nil {
		return 0, fmt.Errorf("timestamp parse: %w", err)
	}

	if timeDiff := time.Now().Unix() - timestamp; timeDiff > timestampLimit || timeDiff < -timestampLimit {
		return 0, fmt.Errorf("not synchronized time, diff=%d, but abs limit=%d", timeDiff, timestampLimit)
	}

	return timestamp, nil
}

func verifyChunk(value []byte) (uint32, error) {
	var chunk uint32

	err := binary.Read(bytes.NewReader(value), binary.BigEndian, &chunk)
	if err != nil {
		return 0, fmt.Errorf("chunk parse: %w", err)
	}

	return chunk, nil
}

// Verify is called by servers, it checks authorization header read from `r` and returns a new token if it's valid.
func Verify(r io.Reader, tokens map[uint16]*Token) (*Token, error) {
	if r == nil {
		return nil, errors.Join(ErrFormat, errors.New("nil reader"))
	}

	header := make([]byte, Size)
	n, err := r.Read(header)

	if err != nil {
		return nil, errors.Join(ErrFormat, fmt.Errorf("failed to read header data: %w", err))
	}

	if n != Size {
		return nil, errors.Join(ErrFormat, fmt.Errorf("invalid token length: %d", n))
	}

	return Parse(header, tokens)
}
