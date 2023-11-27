package auth

import (
	"fmt"
	"maps"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAuthorize(t *testing.T) {
	testCases := []struct {
		name       string
		tokens     map[string]struct{}
		authHeader string
		want       bool
	}{
		{
			name:   "empty",
			tokens: map[string]struct{}{},
			want:   true,
		},
		{
			name:       "valid",
			tokens:     map[string]struct{}{"valid-token": {}},
			authHeader: "Bearer valid-token",
			want:       true,
		},
		{
			name:       "invalid",
			tokens:     map[string]struct{}{"valid-token": {}},
			authHeader: "Bearer invalid-token",
		},
		{
			name:       "failed_format",
			tokens:     map[string]struct{}{"valid-token": {}},
			authHeader: "Basic valid-token",
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)

			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}

			if got := Authorize(tc.tokens, req); got != tc.want {
				t.Errorf("Authorize() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLoadTokens(t *testing.T) {
	testCases := []struct {
		tokens   string
		expected map[string]struct{}
	}{
		{tokens: "", expected: map[string]struct{}{}},
		{tokens: "token1", expected: map[string]struct{}{"token1": {}}},
		{tokens: "token1,token2", expected: map[string]struct{}{"token1": {}, "token2": {}}},
		{tokens: " token1,,token2, ,", expected: map[string]struct{}{"token1": {}, "token2": {}}},
		{tokens: ",,token2, ,", expected: map[string]struct{}{"token2": {}}},
		{tokens: "token1,token2, ,token3 ", expected: map[string]struct{}{"token1": {}, "token2": {}, "token3": {}}},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			err := os.Setenv(ServerEnv, tc.tokens)
			if err != nil {
				t.Fatalf("failed to set environment variable: %v", err)
			}

			defer func() {
				if e := os.Unsetenv(ServerEnv); e != nil {
					t.Errorf("failed to unset environment variable: %v", e)
				}
			}()

			if got := LoadTokens(); !maps.Equal(got, tc.expected) {
				t.Errorf("LoadTokens() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestToken(t *testing.T) {
	err := os.Setenv(clientEnv, "client-token")
	if err != nil {
		t.Fatalf("failed to set environment variable: %v", err)
	}

	defer func() {
		if e := os.Unsetenv(clientEnv); e != nil {
			t.Errorf("failed to unset environment variable: %v", e)
		}
	}()

	want := "client-token"
	if got := Token(); got != want {
		t.Errorf("Token() = %v, want %v", got, want)
	}
}
