package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

func TestNew(t *testing.T) {
	const timeout = 3 * time.Second

	testCases := []struct {
		name      string
		host      string
		port      uint64
		withError bool
	}{
		{name: "valid", host: "localhost", port: 18081},
		{name: "invalid_port", host: "localhost", withError: true},
		{name: "failed_port", host: "localhost", port: 100_000, withError: true},
		{name: "empty_host", port: 18081},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			s, err := New(tc.host, tc.port, timeout)

			if (err != nil) != tc.withError {
				t.Errorf("want error %v, got %v", tc.withError, err)
			}

			if err == nil && s == nil {
				t.Error("want server, got nil")
			}
		})
	}
}

func TestDownload(t *testing.T) {
	s, err := New("localhost", 18081, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	req := httptest.NewRequest("GET", "http://localhsot/download", nil)
	w := httptest.NewRecorder()

	if err = s.download(w, req); err != nil {
		t.Errorf("failed to download: %v", err)
	}
}

func TestUpload(t *testing.T) {
	s, err := New("localhost", 18082, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	body := strings.NewReader("test")
	req := httptest.NewRequest("POST", "http://localhsot/upload", body)
	w := httptest.NewRecorder()

	if err = s.upload(w, req); err != nil {
		t.Errorf("failed to upload: %v", err)
	}
}

func TestServer_Start(t *testing.T) {
	s, err := New("localhost", 18083, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err = s.Start(ctx); err != nil {
		t.Errorf("failed to start: %v", err)
	}
}

func TestServer_Handle(t *testing.T) {
	handlers := map[string]handlerType{
		common.UploadURL: func(w http.ResponseWriter, r *http.Request) error {
			var buffer [32]byte

			n, err := r.Body.Read(buffer[:])
			if err != nil {
				if !errors.Is(err, io.EOF) {
					return fmt.Errorf("failed to read body data: %w", err)
				}
			}

			if n == 0 {
				return errors.New("uploaded zero bytes")
			}

			if err = r.Body.Close(); err != nil {
				return fmt.Errorf("failed to close body: %w", err)
			}

			w.WriteHeader(http.StatusOK)
			return nil
		},
		common.DownloadURL: func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte{0, 1})

			if err != nil {
				return fmt.Errorf("failed to write data: %w", err)
			}
			return nil
		},
	}

	server := httptest.NewServer(http.HandlerFunc(rootHandler(nil, handlers)))
	defer server.Close()
	client := server.Client()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+common.DownloadURL, nil)
	if err != nil {
		t.Fatalf("failed to create download request: %v", err)
	}

	_, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to download: %v", err)
	}

	req, err = http.NewRequestWithContext(ctx, "POST", server.URL+common.UploadURL, strings.NewReader("test"))
	if err != nil {
		t.Fatalf("failed to create upload request: %v", err)
	}

	_, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to upload: %v", err)
	}
}

func TestServer_Token(t *testing.T) {
	err := os.Setenv(auth.ServerEnv, "token1,token2")
	if err != nil {
		t.Fatalf("failed to set environment variable: %v", err)
	}

	defer func() {
		if e := os.Unsetenv(auth.ServerEnv); e != nil {
			t.Errorf("failed to unset environment variable: %v", e)
		}
	}()

	s, err := New("localhost", 18084, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client := &http.Client{}

	go func() {
		defer cancel()
		time.Sleep(50 * time.Millisecond) // wait for server start

		req, e := http.NewRequest("GET", "http://localhost:18084/download", nil)
		if e != nil {
			t.Errorf("failed to create download request: %v", e)
			return
		}

		resp, e := client.Do(req)
		if e != nil {
			t.Errorf("failed to download: %v", e)
			return
		}

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("want %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}

		req, e = http.NewRequest("GET", "http://localhost:18084/download", nil)
		if e != nil {
			t.Errorf("failed to create download request: %v", e)
			return
		}

		req.Header.Add(auth.AuthorizationHeader, auth.Prefix+"token1")
		if resp, e = client.Do(req); e != nil {
			t.Errorf("failed to download: %v", e)
			return
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("want %d, got %d", http.StatusUnauthorized, resp.StatusCode)
		}
	}()

	if err = s.Start(ctx); err != nil {
		t.Errorf("failed to start: %v", err)
	}
}
