package token

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

var (
	firstTimestamp = map[int]byte{
		endIP:     0x00,
		endIP + 1: 0x00,
		endIP + 2: 0x00,
		endIP + 3: 0x00,
		endIP + 4: 0x00,
		endIP + 5: 0x00,
		endIP + 6: 0x00,
		endIP + 7: 0x01,
	}
	commonSecret = []byte{0x33, 0x12, 0xa1, 0x8b}
)

type failedWriter struct {
	length int
}

func (fw *failedWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("test")
}

func (fw *failedWriter) Read(_ []byte) (int, error) {
	return fw.length, nil
}

type failedReader struct {
	length int
}

func (fr *failedReader) Write(_ []byte) (int, error) {
	return fr.length, nil
}

func (fr *failedReader) Read(_ []byte) (int, error) {
	return 0, errors.New("test")
}

type testReadWriter struct {
	lengthW int
	lengthR int
}

func (trw *testReadWriter) Write(_ []byte) (int, error) {
	return trw.lengthW, nil
}

func (trw *testReadWriter) Read(_ []byte) (int, error) {
	return trw.lengthR, nil
}

func buildToken(secret []byte, changes map[int]byte) []byte {
	value := make([]byte, Size)
	value[1] = 1 // clientID

	timestamp := time.Now().Unix()
	binary.BigEndian.PutUint64(value[endIP:], uint64(timestamp))

	for i, v := range changes {
		value[i] = v
	}

	if secret != nil {
		prefixPart := value[:endPayload]

		h := sha512.New512_256()
		h.Write(prefixPart)
		h.Write(secret)

		copy(value[endPayload:], h.Sum(nil))
	}

	return value
}

func buildReader(secret []byte, changes map[int]byte) io.Reader {
	value := buildToken(secret, changes)
	return bytes.NewReader(value)
}

func TestVerify(t *testing.T) {
	testCases := []struct {
		name      string
		tokens    map[uint16]*Token
		reader    io.Reader
		errSubstr string
	}{
		{
			name:      "empty_tokens",
			errSubstr: "nil reader",
		},
		{
			name: "empty_reader",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader:    bytes.NewReader(nil),
			errSubstr: "failed to read header data: ",
		},
		{
			name: "invalid_length", // reader length is 3 bytes, what less than Size
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader:    bytes.NewReader([]byte{0x01, 0x02, 0x03}),
			errSubstr: "invalid token length",
		},
		{
			name: "invalid_client_id", // clientID is 2, but token has only for clientID=1
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader:    buildReader(nil, map[int]byte{1: 0x02}),
			errSubstr: "unknown clientID",
		},
		{
			name: "invalid_timestamp", // timestamp is 1, but test time is time.Now().Unix()
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader:    buildReader(nil, firstTimestamp),
			errSubstr: "not synchronized time",
		},
		{
			name: "invalid_signature", // reader's secret is nil, but token has another value
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader:    buildReader(nil, nil),
			errSubstr: "invalid token signature",
		},
		{
			name: "valid",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			reader: buildReader(commonSecret, nil),
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			token, err := Verify(tc.reader, tc.tokens)
			if err != nil {
				if tc.errSubstr == "" {
					t.Errorf("Verify() error = %v, want nil", err)
					return
				}

				if errStr := err.Error(); !strings.Contains(errStr, tc.errSubstr) {
					t.Errorf("Verify() error = %v, want %v", errStr, tc.errSubstr)
				}
				return
			} else {
				if token == nil {
					if len(tc.tokens) != 0 {
						t.Error("Verify() token = nil")
					}
					return
				}

				if !token.Equal(tc.tokens[1]) {
					t.Errorf("Verify() token = %v, want %v", token, tc.tokens[1])
				}
			}

		})
	}
}

func TestParse(t *testing.T) {
	testCases := []struct {
		name      string
		value     []byte
		tokens    map[uint16]*Token
		errSubstr string
	}{
		{
			name: "empty_value",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			errSubstr: "invalid value length",
		},
		{
			name:  "unknown_client_id",
			value: buildToken(nil, map[int]byte{1: 0x02}),
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			errSubstr: "unknown clientID: 2",
		},
		{
			name:  "invalid_timestamp",
			value: buildToken(nil, firstTimestamp),
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			errSubstr: "not synchronized time",
		},
		{
			name:  "invalid_signature",
			value: buildToken(nil, nil),
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
			errSubstr: "invalid token signature",
		},
		{
			name:  "valid",
			value: buildToken(commonSecret, nil),
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: commonSecret},
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			token, err := Parse(tc.value, tc.tokens)
			if err != nil {
				if tc.errSubstr == "" {
					t.Errorf("Parse() error = %v, want nil", err)
					return
				}

				if errStr := err.Error(); !strings.Contains(errStr, tc.errSubstr) {
					t.Errorf("Parse() error = %v, want %v", errStr, tc.errSubstr)
				}
				return
			} else {
				if token == nil {
					if len(tc.tokens) != 0 {
						t.Error("Parse() token = nil")
					}
					return
				}

				if !token.Equal(tc.tokens[1]) {
					t.Errorf("Parse() token = %v, want %v", token, tc.tokens[1])
				}
			}

		})
	}
}

func TestToken_Verify(t *testing.T) {
	testCases := []struct {
		name           string
		token          *Token
		touchSecret    bool
		touchClientID  bool
		touchSalt      bool
		touchTimestamp bool
	}{
		{
			name:  "valid",
			token: &Token{ClientID: 10, Secret: commonSecret},
		},
		{
			name:        "invalid_secret",
			token:       &Token{ClientID: 10, Secret: commonSecret},
			touchSecret: true,
		},
		{
			name:          "invalid_client_id",
			token:         &Token{ClientID: 10, Secret: commonSecret},
			touchClientID: true,
		},
		{
			name:      "invalid_salt",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			touchSalt: true,
		},
		{
			name:           "invalid_timestamp",
			token:          &Token{ClientID: 10, Secret: commonSecret},
			touchTimestamp: true,
		},
		{
			name:           "invalid",
			token:          &Token{ClientID: 10, Secret: commonSecret},
			touchSecret:    true,
			touchClientID:  true,
			touchSalt:      true,
			touchTimestamp: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.token.Sign()
			if err != nil {
				t.Error(err)
				return
			}

			if tc.touchSecret {
				tc.token.Secret[0] = tc.token.Secret[0] << 1
			}

			if tc.touchClientID {
				tc.token.ClientID += 1
			}

			if tc.touchSalt {
				tc.token.salt[0] = ^tc.token.salt[0]
			}

			if tc.touchTimestamp {
				tc.token.timestamp -= 1
			}

			offset := len(data) - sha512.Size256
			signature := data[offset:]

			err = tc.token.Verify(signature)
			withError := tc.touchSecret || tc.touchClientID || tc.touchSalt || tc.touchTimestamp

			if (err != nil) != withError {
				t.Errorf("Verify() = %v, want %v", err, !withError)
			}
		})
	}
}

func TestToken_Build(t *testing.T) {
	// useless, just for coverage
	token := &Token{ClientID: 1, Secret: commonSecret}

	data, err := token.Build()
	if err != nil {
		t.Fatal(err)
	}

	prefixPart := data[:endPayload]

	h := sha512.New512_256()
	h.Write(prefixPart)
	h.Write(token.Secret)

	if signature := h.Sum(nil); !bytes.Equal(signature, token.signature[:]) {
		t.Errorf("Build() = %v, want %v", signature, token.signature[:])
	}
}

func TestToken_Handshake(t *testing.T) {
	testCases := []struct {
		name      string
		token     *Token
		rw        io.ReadWriter
		errSubstr string
	}{
		{
			name:      "empty",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			errSubstr: "nil reader/writer",
		},
		{
			name:      "failed_write",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			rw:        &failedWriter{},
			errSubstr: "failed to write header data:",
		},
		{
			name:      "failed_write_length",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			rw:        &testReadWriter{lengthW: Size + 1},
			errSubstr: "invalid write token length",
		},
		{
			name:      "failed_read",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			rw:        &failedReader{Size},
			errSubstr: "failed to read header data:",
		},
		{
			name:      "failed_read_length",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			rw:        &testReadWriter{lengthW: Size, lengthR: Size + 1},
			errSubstr: "invalid read token length",
		},
		{
			name:      "unknown_client_id",
			token:     &Token{ClientID: 10, Secret: commonSecret},
			rw:        &testReadWriter{lengthW: Size, lengthR: Size},
			errSubstr: "unknown clientID",
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := tc.token.Handshake(tc.rw)
			if err != nil {
				if tc.errSubstr == "" {
					t.Errorf("Handshake() error = %v, want nil", err)
					return
				}

				if errStr := err.Error(); !strings.Contains(errStr, tc.errSubstr) {
					t.Errorf("Handshake() error = %v, want %v", errStr, tc.errSubstr)
				}
				return
			}
		})
	}
}

func TestToken_Action(t *testing.T) {
	var token Token

	if action := token.Action(); action != actionUpload {
		t.Errorf("Action() = %v, want %v", action, actionUpload)
	}

	token.Download = true
	if action := token.Action(); action != actionDownload {
		t.Errorf("Action() = %v, want %v", action, actionDownload)
	}
}

func TestToken_Stop(t *testing.T) {
	var token Token

	if stop := token.Stop(); !stop {
		t.Error("Stop() = false, want true")
	}

	token.Chunk = 10
	if stop := token.Stop(); stop {
		t.Error("Stop() = true, want false")
	}
}
