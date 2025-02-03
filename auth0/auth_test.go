package auth0

import (
	"fmt"
	"maps"
	"os"
	"testing"

	"github.com/z0rr0/spts/auth0/token"
)

func compare(a, b map[uint16]*token.Token) error {
	eq := maps.EqualFunc(a, b, func(v1, v2 *token.Token) bool { return v1.Equal(v2) })

	if !eq {
		return fmt.Errorf("tokens are not equal")
	}

	return nil
}

func TestServerTokens(t *testing.T) {
	testCases := []struct {
		name     string
		tokens   string
		expected map[uint16]*token.Token
		withErr  bool
	}{
		{
			name:    "empty",
			tokens:  "",
			withErr: true,
		},
		{
			name:     "valid",
			tokens:   "1:3312a18b",
			expected: map[uint16]*token.Token{1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}}},
		},
		{
			name:   "valid_multiple",
			tokens: "1:3312a18b,2:666bf6a2",
			expected: map[uint16]*token.Token{
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

			if err = compare(m, tc.expected); err != nil {
				t.Errorf("ServerTokens() = %v", err)
			}
		})
	}
}

func TestNewToken(t *testing.T) {
	testCases := []struct {
		name    string
		pair    string
		want    *token.Token
		withErr bool
	}{
		{
			name:    "empty",
			withErr: true,
		},
		{
			name: "valid",
			pair: "1:3312a18b",
			want: &token.Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
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
			newToken, err := NewToken(tc.pair, 0)
			if (err != nil) != tc.withErr {
				t.Fatalf("NewToken() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if !newToken.Equal(tc.want) {
				t.Errorf("NewToken() = %v, want %v", newToken, tc.want)
			}
		})
	}
}

func TestClientToken(t *testing.T) {
	testCases := []struct {
		name    string
		value   string
		token   *token.Token
		withErr bool
	}{
		{
			name:    "empty",
			withErr: true,
		},
		{
			name:  "valid",
			value: "1:3312a18b",
			token: &token.Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
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

			clientToken, err := ClientToken()

			if (err != nil) != tc.withErr {
				t.Fatalf("ClientToken() error = %v, wantErr %v", err, tc.withErr)
			}

			if err != nil {
				return
			}

			if !clientToken.Equal(tc.token) {
				t.Errorf("ClientToken() = %v, want %v", clientToken, tc.token)
			}
		})
	}
}
