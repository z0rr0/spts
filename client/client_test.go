package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_Download(t *testing.T) {
	var data = []byte("test data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.WriteHeader(http.StatusOK)
			_, err := w.Write(data)

			if err != nil {
				t.Errorf("Failed to write data: %v", err)
			}
		}
	}))
	defer server.Close()

	client, err := New(server.URL, 8080, 50*time.Millisecond, true)

	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count, err := client.download(ctx, server.Client())
	if err != nil {
		t.Errorf("failed download: %v", err)
	}

	if want := int64(len(data)); count != want {
		t.Errorf("want %d, got %d", want, count)
	}
}

func TestClient_Upload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/upload") {
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

			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, 8080, 100*time.Millisecond, true)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	count, err := client.upload(ctx, server.Client())
	if err != nil {
		t.Errorf("failed upload: %v", err)
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

	client, err := New(server.URL, 8080, 100*time.Millisecond, true)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	client.address = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// address already has properly server URL
	//ctx = context.WithValue(ctx, ctxClientKey, server.Client())

	var b bytes.Buffer
	ctx = context.WithValue(ctx, ctxWriterKey, &b)

	err = client.Start(ctx)
	if err != nil {
		t.Errorf("failed to start client: %v", err)
	}

	lines := strings.Split(b.String(), "\n")
	if n := len(lines); n != 3 {
		t.Errorf("want 3 lines, got %d", n)
	}

	if !strings.HasPrefix(lines[0], "Download speed:") {
		t.Error("failed prefix for download")
	}

	if !strings.HasPrefix(lines[1], "Upload speed:") {
		t.Error("failed prefix for upload")
	}

	client.noDot = false
	err = client.Start(ctx)
	if err != nil {
		t.Errorf("failed to start client: %v", err)
	}
}

func TestClient_String(t *testing.T) {
	client, err := New("localhost", 8080, 100*time.Millisecond, false)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	expected := "\n"
	if got := client.newLine(); got != expected {
		t.Errorf("want %s, got %s", expected, got)
	}

	expected = "address: http://localhost:8080, timeout: 100ms"
	if got := client.String(); got != expected {
		t.Errorf("want %q, got %q", expected, got)
	}
}
