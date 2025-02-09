package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Override(t *testing.T) {
	tests := []struct {
		name     string
		initial  *Config
		params   *Config
		expected *Config
	}{
		{
			name: "nil params",
			initial: &Config{
				Host: "localhost",
				Port: 8080,
			},
			params: nil,
			expected: &Config{
				Host: "localhost",
				Port: 8080,
			},
		},
		{
			name: "override all fields",
			initial: &Config{
				Host:     "localhost",
				Port:     8080,
				Debug:    false,
				Database: "db1",
				Secret:   "secret1",
			},
			params: &Config{
				Host:     "127.0.0.1",
				Port:     9090,
				Debug:    true,
				Database: "db2",
				Secret:   "secret2",
			},
			expected: &Config{
				Host:     "127.0.0.1",
				Port:     9090,
				Debug:    true,
				Database: "db2",
				Secret:   "secret2",
			},
		},
		{
			name: "override partial fields",
			initial: &Config{
				Host:  "localhost",
				Port:  8080,
				Debug: false,
			},
			params: &Config{
				Host:  "127.0.0.1",
				Debug: true,
			},
			expected: &Config{
				Host:  "127.0.0.1",
				Port:  8080,
				Debug: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initial.override(tt.params)
			assert.Equal(t, tt.expected, tt.initial)
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	validDBPath := filepath.Join(tmpDir, "test.db")

	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Host:     "localhost",
				Port:     8080,
				Secret:   "secret",
				Database: validDBPath,
			},
		},
		{
			name: "empty host",
			config: &Config{
				Port:     8080,
				Secret:   "secret",
				Database: validDBPath,
			},
			expectError: true,
		},
		{
			name: "zero port",
			config: &Config{
				Host:     "localhost",
				Secret:   "secret",
				Database: validDBPath,
			},
			expectError: true,
		},
		{
			name: "empty secret",
			config: &Config{
				Host:     "localhost",
				Port:     8080,
				Database: validDBPath,
			},
			expectError: true,
		},
		{
			name: "invalid database path",
			config: &Config{
				Host:     "localhost",
				Port:     8080,
				Secret:   "secret",
				Database: "/invalid/path/db",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "test.txt")

	writeErr := os.WriteFile(validFile, []byte("test"), 0640)
	require.NoError(t, writeErr)

	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name: "valid temp path",
			path: validFile,
		},
		{
			name: "valid docker path",
			path: "/data/test.txt",
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
		},
		{
			name:        "invalid absolute path",
			path:        "/etc/passwd",
			expectError: true,
		},
		{
			name: "path with spaces",
			path: "  test.txt  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateFilePath(tt.path)
			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, result)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	dbPath := filepath.Join(tmpDir, "test.db")

	validConfig := `
host = "localhost"
port = 8080
debug = false
production = false
database = "` + dbPath + `"
secret = "test-secret"
`
	writeErr := os.WriteFile(configPath, []byte(validConfig), 0644)
	require.NoError(t, writeErr)

	tests := []struct {
		name        string
		fileName    string
		params      *Config
		expectError bool
	}{
		{
			name:     "valid config file",
			fileName: configPath,
			params:   nil,
		},
		{
			name:     "override from params",
			fileName: configPath,
			params: &Config{
				Host: "127.0.0.1",
				Port: 9090,
			},
		},
		{
			name:        "invalid config file",
			fileName:    "nonexistent.toml",
			params:      nil,
			expectError: true,
		},
		{
			name:     "no file, valid params",
			fileName: "",
			params: &Config{
				Host:     "localhost",
				Port:     8080,
				Secret:   "secret",
				Database: dbPath,
			},
		},
		{
			name:     "no file, invalid params",
			fileName: "",
			params: &Config{
				Host: "localhost",
				// missing required fields
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := New(tt.fileName, tt.params)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cfg)

				assert.Empty(t, cfg.Secret)
				assert.NotEmpty(t, cfg.SecretBytes)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "test.txt")

	testContent := []byte("test content")
	writeErr := os.WriteFile(validFile, testContent, 0644)
	require.NoError(t, writeErr)

	tests := []struct {
		name        string
		path        string
		expectError bool
		expected    []byte
	}{
		{
			name:     "valid file",
			path:     validFile,
			expected: testContent,
		},
		{
			name:        "nonexistent file",
			path:        filepath.Join(tmpDir, "nonexistent.txt"),
			expectError: true,
		},
		{
			name:        "invalid path",
			path:        "/etc/passwd",
			expectError: true,
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := ReadFile(tt.path)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, content)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, content)
			}
		})
	}
}
