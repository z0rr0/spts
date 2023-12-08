package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

func TestClient_Download(t *testing.T) {
	const clientIP = "123.124.125.126"
	var data = []byte("test data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set(common.XRequestIPHeader, clientIP)
			w.WriteHeader(http.StatusOK)

			_, err := w.Write(data)

			if err != nil {
				t.Errorf("Failed to write data: %v", err)
			}
		}
	}))
	defer server.Close()

	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 50 * time.Millisecond, Dot: true}
	client, err := New(params)

	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.Address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token, _ := auth.ClientToken()
	count, ip, err := client.download(ctx, token, server.Client())

	if err != nil {
		t.Errorf("failed download: %v", err)
	}

	if ip != clientIP {
		t.Errorf("want %s, got %s IP address", clientIP, ip)
	}

	if want := int64(len(data)); count != want {
		t.Errorf("want %d, got %d", want, count)
	}
}

func TestClient_Upload(t *testing.T) {
	const clientIP = "123.124.125.126"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/upload") {
			w.Header().Set(common.XRequestIPHeader, clientIP)
			w.WriteHeader(http.StatusOK)

			buffer := make([]byte, 32)

			n, err := r.Body.Read(buffer)
			if err != nil {
				t.Errorf("failed to read body data: %v", err)
			}

			if n == 0 {
				t.Error("uploaded zero bytes")
			}

			if err = r.Body.Close(); err != nil {
				t.Errorf("failed to close body: %v", err)
			}
		}
	}))
	defer server.Close()

	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 100 * time.Millisecond, Dot: true}
	client, err := New(params)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.Address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	token, _ := auth.ClientToken()
	count, ip, err := client.upload(ctx, token, server.Client())

	if err != nil {
		t.Errorf("failed upload: %v", err)
	}

	if ip != clientIP {
		t.Errorf("want %s, got %s IP address", clientIP, ip)
	}

	if count == 0 {
		t.Error("no uploaded data")
	}
}

func TestClient_Start(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte{0, 1})

			if err != nil {
				t.Errorf("Failed to write data: %v", err)
			}
		}

		if strings.HasSuffix(r.URL.Path, "/upload") {
			buffer := make([]byte, 2)
			if _, err := r.Body.Read(buffer); err != nil {
				t.Errorf("failed to read body data: %v", err)
			}

			if err := r.Body.Close(); err != nil {
				t.Errorf("failed to close body: %v", err)
			}
		}
	}))
	defer server.Close()

	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 100 * time.Millisecond}
	client, err := New(params)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.Address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var b bytes.Buffer
	ctx = context.WithValue(ctx, ctxWriterKey, &b)

	err = client.Start(ctx)
	if err != nil {
		t.Errorf("failed to start client: %v", err)
	}

	lines := strings.Split(b.String(), "\n")
	if n := len(lines); n != 4 {
		// IP, download, upload, empty line
		t.Errorf("want 3 lines, got %d: %q", n, lines)
	}

	if !strings.HasPrefix(lines[0], "IP address:") {
		t.Error("failed prefix for IP address")
	}

	if !strings.HasPrefix(lines[1], "Download speed:") {
		t.Error("failed prefix for download")
	}

	if !strings.HasPrefix(lines[2], "Upload speed:") {
		t.Error("failed prefix for upload")
	}

	client.Dot = false
	err = client.Start(ctx)
	if err != nil {
		t.Errorf("failed to start client: %v", err)
	}
}

func TestClient_String(t *testing.T) {
	params := &common.Params{Host: "localhost", Port: 8080, Timeout: 100 * time.Millisecond, Dot: true}
	client, err := New(params)

	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	expected := "\n"
	if got := client.NewLine(); got != expected {
		t.Errorf("want %s, got %s", expected, got)
	}

	expected = "address: http://localhost:8080, timeout: 100ms"
	if got := client.String(); got != expected {
		t.Errorf("want %q, got %q", expected, got)
	}
}

func TestClient_Token(t *testing.T) {
	serverTokens := map[uint16]*auth.Token{
		1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
	}
	clientToken := &auth.Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}} // client #1

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := auth.Authorize(r, serverTokens); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		buffer := make([]byte, 2)
		if _, err := r.Body.Read(buffer); err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("failed to read body data: %v", err)
		}

		if err := r.Body.Close(); err != nil {
			t.Errorf("failed to close body: %v", err)
		}

		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte{0, 1})

		if err != nil {
			t.Errorf("Failed to write data: %v", err)
		}
	}))
	defer server.Close()

	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 50 * time.Millisecond, Dot: true}
	client, err := New(params)

	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.Address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, _, err = client.download(ctx, clientToken, server.Client())
	if err != nil {
		t.Errorf("failed download: %v", err)
	}

	_, _, err = client.upload(ctx, clientToken, server.Client())
	if err != nil {
		t.Errorf("failed upload: %v", err)
	}

	clientToken.ClientID = 3 // unknown client
	_, _, err = client.download(ctx, clientToken, server.Client())
	if err == nil {
		t.Errorf("error expected")
	}
}
