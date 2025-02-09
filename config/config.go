package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// dockerDir is a directory for docker volumes.
const dockerDir = "/data"

var (
	// ErrValidation is an error for configuration validation.
	ErrValidation = fmt.Errorf("config validation failed")

	// ErrFileRead is an error for file read.
	ErrFileRead = fmt.Errorf("read file error")
)

// Duration is a wrapper around time.Duration that supports unmarshalling from a JSON string.
type Duration time.Duration

// UnmarshalText parses a TOML string into a Duration type.
func (d *Duration) UnmarshalText(b []byte) error {
	s := string(b)

	duration, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("failed to parse duration [%s]: %w", s, err)
	}

	*d = Duration(duration)
	return nil
}

// Timed returns a time.Duration value of the Duration type.
func (d *Duration) Timed() time.Duration {
	return time.Duration(*d)
}

// Config is a configuration structure.
type Config struct {
	Host          string   `toml:"host"`
	Port          uint16   `toml:"port"`
	Debug         bool     `toml:"debug"`
	Production    bool     `toml:"production"`
	Database      string   `toml:"database"`
	TokenDuration Duration `toml:"token_duration"`
	Secret        string   `toml:"secret"`
	Salt          string   `toml:"salt"`
	SecretBytes   []byte   `toml:"-"`
	SaltBytes     []byte   `toml:"-"`
}

// override updates configuration fields from the `params`.
func (c *Config) override(params *Config) {
	if params == nil {
		return
	}

	c.Debug = c.Debug || params.Debug
	c.Production = c.Production || params.Production

	if params.Host != "" {
		c.Host = params.Host
	}
	if params.Port > 0 {
		c.Port = params.Port
	}
	if params.Database != "" {
		c.Database = params.Database
	}
	if len(params.Secret) > 0 {
		c.Secret = params.Secret
	}
	if len(params.Salt) > 0 {
		c.Salt = params.Salt
	}
	if params.TokenDuration > 0 {
		c.TokenDuration = params.TokenDuration
	}
}

// Validate checks configuration fields.
func (c *Config) Validate() error {
	var err error

	if c.Host == "" {
		err = errors.Join(err, fmt.Errorf("host is empty"))
	}
	if c.Port == 0 {
		err = errors.Join(err, fmt.Errorf("port is empty"))
	}
	if len(c.Secret) == 0 {
		err = errors.Join(err, fmt.Errorf("secret is empty"))
	}
	if len(c.Salt) == 0 {
		err = errors.Join(err, fmt.Errorf("salt is empty"))
	}
	if c.TokenDuration == 0 {
		err = errors.Join(err, fmt.Errorf("token duration is empty"))
	}
	if c.Database == "" {
		err = errors.Join(err, fmt.Errorf("database is empty"))
	}

	if cleanDatabasePath, dbError := validateFilePath(c.Database); dbError != nil {
		dbError = errors.Join(ErrFileRead, dbError)
		err = errors.Join(err, dbError)
	} else {
		c.Database = cleanDatabasePath
	}

	if err != nil {
		return err
	}

	return nil
}

// newFromFile reads a configuration file and returns a new configuration.
func newFromFile(fileName string) (*Config, error) {
	data, err := ReadFile(fileName)
	if err != nil {
		return nil, errors.Join(ErrFileRead, fmt.Errorf("read file: %w", err))
	}

	cfg := &Config{}
	if err = toml.Unmarshal(data, cfg); err != nil {
		return nil, errors.Join(ErrValidation, fmt.Errorf("unmarshal toml: %w", err))
	}

	return cfg, nil
}

// New reads a configuration file and returns a new configuration.
// The argument `params` is configuration parameters from command line.
func New(fileName string, params *Config) (*Config, error) {
	var (
		cfg *Config
		err error
	)

	if fileName != "" {
		if cfg, err = newFromFile(fileName); err != nil {
			return nil, err
		}
		cfg.override(params)
	} else {
		cfg = params
	}

	if err = cfg.Validate(); err != nil {
		return nil, errors.Join(ErrValidation, err)
	}

	cfg.SecretBytes = []byte(cfg.Secret)
	cfg.Secret = "" // clear secret

	cfg.SaltBytes = []byte(cfg.Salt)
	cfg.Salt = "" // clear salt

	return cfg, nil
}

// validateFilePath checks if the file path is valid and safe.
func validateFilePath(fileName string) (string, error) {
	if fileName == "" {
		return "", errors.New("file name is empty")
	}

	cleanPath := filepath.Clean(strings.Trim(fileName, " "))
	if filepath.IsAbs(cleanPath) {
		tmpDir := os.TempDir()
		if !(strings.HasPrefix(cleanPath, dockerDir) || strings.HasPrefix(cleanPath, tmpDir)) {
			return "", fmt.Errorf("file %q has invalid path", cleanPath)
		}

		fileName = cleanPath
	} else {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get current dir: %w", err)
		}

		fileName = filepath.Join(currentDir, cleanPath)
	}

	return fileName, nil
}

// ReadFile reads a file from the current directory or from the /data (docker) or temporary directory.
func ReadFile(fileName string) ([]byte, error) {
	cleanPath, err := validateFilePath(fileName)
	if err != nil {
		return nil, err
	}

	return os.ReadFile(cleanPath)
}
