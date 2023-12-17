package auth

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func compareTokens(a, b map[uint16]*Token) error {
	if n, m := len(a), len(b); n != m {
		return fmt.Errorf("different length: %d != %d", n, m)
	}

	for k, v := range a {
		t, ok := b[k]

		if !ok {
			return fmt.Errorf("missing key: %d", k)
		}

		if !v.Equal(t) {
			return fmt.Errorf("different token: %v != %v", v, t)
		}
	}

	return nil
}

func TestLoadTokens(t *testing.T) {
	testCases := []struct {
		name     string
		tokens   string
		expected map[uint16]*Token
		withErr  bool
	}{
		{
			name:     "empty",
			tokens:   "",
			expected: nil,
		},
		{
			name:     "valid",
			tokens:   "1:3312a18b",
			expected: map[uint16]*Token{1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}}},
		},
		{
			name:   "valid_multiple",
			tokens: "1:3312a18b,2:666bf6a2",
			expected: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
				2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
			},
		},
		{
			name:    "invalid",
			tokens:  "1:invalid",
			withErr: true,
		},
		{
			name:    "invalid_multiple",
			tokens:  "1:3312a18b,2:invalid",
			withErr: true,
		},
		{
			name:    "invalid_format",
			tokens:  "1:3312a18b,invalid",
			withErr: true,
		},
		{
			name:    "invalid_client_id_format",
			tokens:  "invalid:3312a18b",
			withErr: true,
		},
		{
			name:    "invalid_client_id_number",
			tokens:  "100000:3312a18b", // clientID - unsigned 16-bit integers (0 to 65535)
			withErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := os.Setenv(ServerEnv, tc.tokens)
			if err != nil {
				t.Fatalf("failed to set environment variable: %v", err)
			}

			defer func() {
				if e := os.Unsetenv(ServerEnv); e != nil {
					t.Errorf("failed to unset environment variable: %v", e)
				}
			}()

			m, err := ServerTokens()
			if (err != nil) != tc.withErr {
				t.Fatalf("ServerTokens() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if err = compareTokens(m, tc.expected); err != nil {
				t.Errorf("ServerTokens() = %v", err)
			}
		})
	}
}

func TestNewToken(t *testing.T) {
	testCases := []struct {
		name    string
		pair    string
		want    *Token
		withErr bool
	}{
		{
			name:    "empty",
			withErr: true,
		},
		{
			name: "valid",
			pair: "1:3312a18b",
			want: &Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		},
		{
			name:    "invalid",
			pair:    "1:invalid",
			withErr: true,
		},
		{
			name:    "invalid_format",
			pair:    "1:3312a18b:invalid",
			withErr: true,
		},
		{
			name:    "invalid_client_id_format",
			pair:    "invalid:3312a18b",
			withErr: true,
		},
		{
			name:    "invalid_client_id_number",
			pair:    "100000:3312a18b", // clientID - unsigned 16-bit integers (0 to 65535)
			withErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			token, err := NewToken(tc.pair)
			if (err != nil) != tc.withErr {
				t.Fatalf("NewToken() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if !token.Equal(tc.want) {
				t.Errorf("NewToken() = %v, want %v", token, tc.want)
			}
		})
	}
}

func testTokenReader(secret []byte, changes map[int]byte) io.Reader {
	header := make([]byte, lenToken)
	header[2] = 1 // clientID

	timestamp := time.Now().Unix()
	binary.BigEndian.PutUint64(header[endSalt:], uint64(timestamp))

	for i, v := range changes {
		header[i] = v
	}

	if secret != nil {
		prefixPart := header[:endTime]

		h := sha512.New()
		h.Write(prefixPart)
		h.Write(secret)

		copy(header[endTime:], h.Sum(nil))
	}

	return bytes.NewReader(header)
}

func TestVerify(t *testing.T) {
	testCases := []struct {
		name      string
		tokens    map[uint16]*Token
		reader    io.Reader
		errSubstr string
	}{
		{
			name: "empty_tokens",
		},
		{
			name: "empty_reader",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader:    bytes.NewReader(nil),
			errSubstr: "failed to read header data: ",
		},
		{
			name: "invalid_length",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader:    bytes.NewReader([]byte{0x01, 0x02, 0x03}),
			errSubstr: "invalid token length",
		},
		{
			name: "invalid_client_id",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader:    testTokenReader(nil, map[int]byte{1: 0x02}),
			errSubstr: "unknown clientID",
		},
		{
			name: "invalid_timestamp",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader: testTokenReader(nil, map[int]byte{
				endSalt:     0x00,
				endSalt + 1: 0x00,
				endSalt + 2: 0x00,
				endSalt + 3: 0x00,
				endSalt + 4: 0x00,
				endSalt + 5: 0x00,
				endSalt + 6: 0x00,
				endSalt + 7: 0x01,
			}),
			errSubstr: "not synchronized time",
		},
		{
			name: "invalid_signature",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader:    testTokenReader(nil, nil),
			errSubstr: "invalid token signature",
		},
		{
			name: "valid",
			tokens: map[uint16]*Token{
				1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			},
			reader: testTokenReader([]byte{0x33, 0x12, 0xa1, 0x8b}, nil),
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
			}

			if token == nil {
				if len(tc.tokens) != 0 {
					t.Error("Verify() token = nil")
				}
				return
			}

			if !token.Equal(tc.tokens[1]) {
				t.Errorf("Verify() token = %v, want %v", token, tc.tokens[1])
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
			token: &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		},
		{
			name:        "invalid_secret",
			token:       &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			touchSecret: true,
		},
		{
			name:          "invalid_client_id",
			token:         &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			touchClientID: true,
		},
		{
			name:      "invalid_salt",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			touchSalt: true,
		},
		{
			name:           "invalid_timestamp",
			token:          &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			touchTimestamp: true,
		},
		{
			name:           "invalid",
			token:          &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			touchSecret:    true,
			touchClientID:  true,
			touchSalt:      true,
			touchTimestamp: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			data := tc.token.Sign()

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

			offset := len(data) - sha512.Size
			signature := data[offset:]

			ok := tc.token.Verify(signature)
			withError := tc.touchSecret || tc.touchClientID || tc.touchSalt || tc.touchTimestamp

			if ok != !withError {
				t.Errorf("Verify() = %v, want %v", ok, !withError)
			}
		})
	}
}

func TestToken_Build(t *testing.T) {
	// useless, just for coverage
	token := &Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}}

	data, err := token.Build()
	if err != nil {
		t.Fatal(err)
	}

	prefixPart := data[:endTime]

	h := sha512.New()
	h.Write(prefixPart)
	h.Write(token.Secret)

	if signature := h.Sum(nil); !bytes.Equal(signature, token.signature[:]) {
		t.Errorf("Build() = %v, want %v", signature, token.signature[:])
	}
}

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

func TestToken_Handshake(t *testing.T) {
	testCases := []struct {
		name      string
		token     *Token
		rw        io.ReadWriter
		errSubstr string
	}{
		{
			name:      "empty",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			errSubstr: "nil reader/writer",
		},
		{
			name:      "failed_write",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			rw:        &failedWriter{},
			errSubstr: "failed to write header data:",
		},
		{
			name:      "failed_write_length",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			rw:        &testReadWriter{lengthW: lenToken + 1},
			errSubstr: "invalid write token length",
		},
		{
			name:      "failed_read",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			rw:        &failedReader{lenToken},
			errSubstr: "failed to read header data:",
		},
		{
			name:      "failed_read_length",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			rw:        &testReadWriter{lengthW: lenToken, lengthR: lenToken + 1},
			errSubstr: "invalid read token length",
		},
		{
			name:      "unknown_client_id",
			token:     &Token{ClientID: 10, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			rw:        &testReadWriter{lengthW: lenToken, lengthR: lenToken},
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

func TestClientToken(t *testing.T) {
	testCases := []struct {
		name    string
		value   string
		token   *Token
		withErr bool
	}{
		{
			name:  "empty",
			token: &Token{},
		},
		{
			name:  "valid",
			value: "1:3312a18b",
			token: &Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		},
		{
			name:    "invalid",
			value:   "1:invalid",
			withErr: true,
		},
		{
			name:    "invalid_format",
			value:   "1:3312a18b:invalid",
			withErr: true,
		},
		{
			name:    "invalid_client_id_format",
			value:   "invalid:3312a18b",
			withErr: true,
		},
		{
			name:    "invalid_client_id_number",
			value:   "100000:3312a18b", // clientID - unsigned 16-bit integers (0 to 65535)
			withErr: true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			err := os.Setenv(ClientEnv, tc.value)
			if err != nil {
				t.Fatalf("failed to set environment variable: %v", err)
			}

			defer func() {
				if e := os.Unsetenv(ClientEnv); e != nil {
					t.Errorf("failed to unset environment variable: %v", e)
				}
			}()

			token, err := ClientToken()

			if (err != nil) != tc.withErr {
				t.Fatalf("ClientToken() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if !token.Equal(tc.token) {
				t.Errorf("ClientToken() = %v, want %v", token, tc.token)
			}
		})
	}
}
