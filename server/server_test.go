package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	s, err := New("localhost", 18081, 20*time.Millisecond)
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
	s, err := New("localhost", 18081, 20*time.Millisecond)
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
