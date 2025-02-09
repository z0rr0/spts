package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/z0rr0/spts/db"
)

const (
	cookieName = "auth"

	authHeader   = "Authorization"
	prefixBasic  = "Basic "
	prefixBearer = "Bearer "
	lenBasic     = len(prefixBasic)
	lenBearer    = len(prefixBearer)
)

var (
	// ErrUserNotFound is an error for user not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrInvalidPassword is an error for invalid password.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrUserExists is an error for user already exists.
	ErrUserExists = errors.New("user already exists")
)

// Config is a main configuration for the authenticator.
type Config struct {
	IsProd        bool
	GlobalSalt    []byte
	JWTSecret     []byte
	TokenDuration time.Duration
}

// Authenticator is an interface for user authentication.
type Authenticator interface {
	Register(username, password string) error
	Update(username, password string) error
	Delete(username string) error
	Verify(username, password string) bool
	UserExists(username string) (bool, error)
}

// UserAuthenticator is an authenticator. It contains base methods for authentication.
type UserAuthenticator struct {
	storage db.UserStorage
	pwdMgr  *PasswordManager
	jwtMgr  *JWTManager
	config  Config
}

// NewAuthenticator creates a new authenticator.
func NewAuthenticator(storage db.UserStorage, config Config) *UserAuthenticator {
	return &UserAuthenticator{
		storage: storage,
		pwdMgr:  NewPasswordManager(config),
		jwtMgr:  NewJWTManager(config),
		config:  config,
	}
}

func (a *UserAuthenticator) UserExists(username string) (bool, error) {
	_, err := a.storage.Get(username)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, db.ErrUserNotFound) {
		return false, nil
	}

	return false, err
}

// Register creates a new user.
func (a *UserAuthenticator) Register(username, password string) error {
	if _, err := a.storage.Get(username); err == nil {
		return ErrUserExists
	}

	salt, err := a.pwdMgr.generateSalt()
	if err != nil {
		return err
	}

	hash, err := a.pwdMgr.Hash(password, salt)
	if err != nil {
		return err
	}

	return a.storage.Add(username, salt, hash)
}

// Update updates user password.
func (a *UserAuthenticator) Update(username, password string) error {
	_, err := a.storage.Get(username)
	if err != nil {
		return ErrUserNotFound
	}

	salt, err := a.pwdMgr.generateSalt()
	if err != nil {
		return err
	}

	hash, err := a.pwdMgr.Hash(password, salt)
	if err != nil {
		return err
	}

	return a.storage.Update(username, salt, hash)
}

// Delete removes user.
func (a *UserAuthenticator) Delete(username string) error {
	_, err := a.storage.Get(username)
	if err != nil {
		return ErrUserNotFound
	}

	return a.storage.Delete(username)
}

// Verify checks if the user exists and password is correct.
func (a *UserAuthenticator) Verify(username, password string) bool {
	user, err := a.storage.Get(username)
	if err != nil {
		return false
	}

	return a.pwdMgr.Verify(password, user.Hash, user.Salt)
}

// Login performs user login, creates a new JWT token.
func (a *UserAuthenticator) Login(username, password string) (string, error) {
	user, err := a.storage.Get(username)
	if err != nil {
		return "", ErrUserNotFound
	}

	if !a.pwdMgr.Verify(password, user.Hash, user.Salt) {
		return "", ErrInvalidPassword
	}

	return a.jwtMgr.Create(username)
}

func (a *UserAuthenticator) SetAuthCookie(w http.ResponseWriter, token string) {
	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(a.config.TokenDuration.Seconds()),
		HttpOnly: true,
		Secure:   a.config.IsProd,
		SameSite: getSameSiteMode(a.config.IsProd),
	}
	http.SetCookie(w, cookie)
}

func getSameSiteMode(isProd bool) http.SameSite {
	if isProd {
		return http.SameSiteStrictMode
	}
	return http.SameSiteLaxMode
}

// BasicAuthMiddleware checks basic auth header and creates a new JWT token.
func (a *UserAuthenticator) BasicAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	const (
		separator  = ":"
		authFields = 2 // username:password
	)
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get(authHeader)
		if !strings.HasPrefix(auth, prefixBasic) {
			http.Error(w, "Authorization required", http.StatusUnauthorized)
			return
		}

		payload, err := base64.StdEncoding.DecodeString(auth[lenBasic:])
		if err != nil {
			slog.Info("failed to decode auth header", "error", err)
			http.Error(w, "Invalid authorization header", http.StatusBadRequest)
			return
		}

		pair := strings.SplitN(string(payload), separator, authFields)
		if n := len(pair); n != authFields {
			slog.Info("invalid auth header", "fields", n)
			http.Error(w, "Invalid authorization header", http.StatusBadRequest)
			return
		}

		token, err := a.Login(pair[0], pair[1])
		if err != nil {
			slog.Info("failed to login", "user", pair[0], "error", err)
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}

		a.SetAuthCookie(w, token)
		next.ServeHTTP(w, r)
	}
}

type userCtxKey string

var userKey = userCtxKey("username")

// JWTMiddleware checks JWT token and adds username to the request context.
func (a *UserAuthenticator) JWTMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var tokenString string

		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenString = auth[lenBearer:]
		} else {
			cookie, err := r.Cookie(cookieName)
			if err != nil {
				slog.Info("failed to get cookie", "error", err)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			tokenString = cookie.Value
		}

		claims, err := a.jwtMgr.Validate(tokenString)
		if err != nil {
			slog.Info("failed to validate token", "error", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userKey, claims.Username)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUsername returns username from the context.
func (a *UserAuthenticator) GetUsername(ctx context.Context) string {
	return ctx.Value(userKey).(string)
}
