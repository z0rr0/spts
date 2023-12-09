package auth

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"os"
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

			m, err := LoadTokens()
			if (err != nil) != tc.withErr {
				t.Fatalf("LoadTokens() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if err = compareTokens(m, tc.expected); err != nil {
				t.Errorf("LoadTokens() = %v", err)
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
			payload := tc.token.String()
			if len(payload) == 0 {
				t.Fatal("zero token length")
			}

			if tc.touchSecret {
				tc.token.Secret[0] = tc.token.Secret[0] << 1
			}

			if tc.touchClientID {
				tc.token.ClientID += 1
			}

			if tc.touchSalt {
				tc.token.Salt[0] = tc.token.Salt[0] << 1
			}

			if tc.touchTimestamp {
				tc.token.timestamp -= 1
			}

			data, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				t.Fatalf("base64 decode: %v", err)
			}

			offset := len(data) - sha512.Size
			signature := data[offset:]

			err = tc.token.Verify(signature)
			withError := tc.touchSecret || tc.touchClientID || tc.touchSalt || tc.touchTimestamp

			if (err != nil) != withError {
				t.Errorf("Verify() error = %v, wantErr %v", err, withError)
			}
		})
	}
}

func TestAuthorize(t *testing.T) {
	var ts = time.Now().Unix()

	serverTokens := map[uint16]*Token{
		1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
	}
	unknownClient := &Token{ClientID: 3, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}}
	failedTs := &Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}, timestamp: ts - timestampLimit - 1}
	failedSign := &Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8c}}

	testCases := []struct {
		name         string
		header       string
		noHeader     bool
		serverTokens map[uint16]*Token
		clientID     uint16
		withError    bool
	}{
		{name: "empty"},
		{
			name:         "no_header",
			noHeader:     true,
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "valid",
			header:       Prefix + serverTokens[1].String(),
			serverTokens: serverTokens,
			clientID:     1,
		},
		{
			name:         "failed_header_prefix",
			header:       "Basic " + serverTokens[1].String(),
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "failed_token_decode",
			header:       Prefix + "invalid",
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "failed_token_length",
			header:       Prefix + "dGVzdAo=", // "test" value
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "failed_unknown_client_id",
			header:       Prefix + unknownClient.String(),
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "failed_timestamp",
			header:       Prefix + failedTs.String(),
			serverTokens: serverTokens,
			withError:    true,
		},
		{
			name:         "failed_signature",
			header:       Prefix + failedSign.String(),
			serverTokens: serverTokens,
			withError:    true,
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if !tc.noHeader {
				r.Header.Set(AuthorizationHeader, tc.header)
			}

			clientID, err := Authorize(r, tc.serverTokens)
			if (err != nil) != tc.withError {
				t.Errorf("Authorize() error = %v, wantErr %v", err, tc.withError)
			}

			if err != nil {
				return
			}

			if clientID != tc.clientID {
				t.Errorf("Authorize clientID %d, want %d", clientID, tc.clientID)
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
