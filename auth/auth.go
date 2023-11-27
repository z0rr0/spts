package auth

import (
	"net/http"
	"os"
	"strings"
)

const (
	// AuthorizationHeader is a header name for authorization.
	AuthorizationHeader = "Authorization"

	// Prefix is a prefix for authorization header.
	Prefix = "Bearer "

	clientEnv = "SPTS_KEY"
	serverEnv = "SPTS_TOKENS"
)

// Authorize checks authorization header and returns true if it is valid.
func Authorize(tokens map[string]struct{}, r *http.Request) bool {
	if len(tokens) == 0 {
		return true // no tokens, authorization is not required
	}

	authorization := r.Header.Get(AuthorizationHeader)
	if authorization == "" {
		return false // no authorization header
	}

	if !strings.HasPrefix(authorization, Prefix) {
		return false // invalid authorization header format
	}

	if _, ok := tokens[strings.TrimPrefix(authorization, Prefix)]; !ok {
		return false // invalid token
	}

	return true // known token
}

// LoadTokens loads server's tokens from environment variable.
func LoadTokens() map[string]struct{} {
	tokens := make(map[string]struct{})

	for _, token := range strings.Split(strings.TrimSpace(os.Getenv(serverEnv)), ",") {
		tokens[token] = struct{}{}
	}

	return tokens
}

// Token returns client's token from environment variable.
func Token() string {
	return strings.TrimSpace(os.Getenv(clientEnv))
}
