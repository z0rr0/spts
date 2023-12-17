package client

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z0rr0/spts/common"
)

const (
	testEnv        = "1:422090c90f7169b4"
	testAccTimeout = 20 * time.Millisecond
)

type testServer struct {
	listener net.Listener
	stop     chan struct{}
}

func (t *testServer) Stop() {
	close(t.stop)
}

func createServer(t *testing.T, f func(conn net.Conn) error) (*testServer, error) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return nil, err
	}

	stop := make(chan struct{})

	go func() {
		defer func() {
			if e := listener.Close(); e != nil {
				t.Errorf("failed to close listener: %v", e)
			}
		}()

		for {
			if e := listener.SetDeadline(time.Now().Add(testAccTimeout)); e != nil {
				t.Errorf("failed to set deadline: %v", e)
				return
			}

			select {
			case <-stop:
				return
			default:
				conn, e := listener.AcceptTCP()
				if e != nil {
					continue
				}

				if e = conn.SetDeadline(time.Now().Add(testAccTimeout * 2)); e != nil {
					t.Errorf("failed to set deadline: %v", e)
					e = errors.Join(e, conn.Close())
					continue
				}

				if e = f(conn); err != nil {
					t.Errorf("failed to handle connection: %v", e)
				}

				if e = conn.Close(); e != nil {
					t.Errorf("failed to close connection: %v", e)
				}
			}
		}
	}()

	return &testServer{listener: listener, stop: stop}, nil
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name      string
		host      string
		port      uint64
		client    string
		errSubstr string
	}{
		{name: "valid", host: "localhost", port: 28082, client: "address: localhost:28082, timeout: 20ms"},
		{name: "invalid_port", host: "localhost", errSubstr: "invalid port"},
		{name: "failed_port", host: "localhost", port: 100_000, errSubstr: "invalid port"},
		{name: "empty_host", port: 28082, errSubstr: "host address is empty"},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			client, err := New(&common.Params{Host: tc.host, Port: tc.port, Timeout: 20 * time.Millisecond, Dot: true})

			if err != nil {
				if tc.errSubstr == "" {
					t.Errorf("want nil, got %v", err)
				}

				if s := err.Error(); !strings.Contains(s, tc.errSubstr) {
					t.Errorf("want %q, got %q", tc.errSubstr, s)
				}
				return
			}

			if s := client.String(); s != tc.client {
				t.Errorf("want %q, got %q", tc.client, s)
			}
		})
	}
}

func TestClient_Download(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	srv, err := createServer(t, func(conn net.Conn) error {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		reader := common.NewReader(ctx)
		buffer := make([]byte, 4*common.KB)

		for {
			n, err := reader.Read(buffer)

			if err != nil {
				t.Fatal(err)
				break
			}

			_, err = conn.Write(buffer[:n])
			if err != nil {
				break
			}

		}

		wg.Done()
		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	client := Client{ServiceBase: common.ServiceBase{Address: srv.listener.Addr().String()}}
	addr := srv.listener.Addr().(*net.TCPAddr)
	conn, err := net.Dial(addr.Network(), addr.String())

	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	count, err := client.download(ctx, conn, addr.IP.String())
	if err != nil {
		t.Errorf("failed download: %v", err)
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}

	t.Logf("downloaded %s bytes", common.ByteSize(count))
	wg.Wait()
}

func TestClient_Upload(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	srv, err := createServer(t, func(conn net.Conn) error {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		count, err := common.Read(ctx, conn, 4*common.KB)
		if err != nil {
			t.Errorf("failed to read server data: %v", err)
		}

		t.Logf("server read %s bytes", common.ByteSize(count))

		wg.Done()
		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	client := Client{ServiceBase: common.ServiceBase{Address: srv.listener.Addr().String()}}
	addr := srv.listener.Addr().(*net.TCPAddr)
	conn, err := net.Dial(addr.Network(), addr.String())

	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	count, err := client.upload(ctx, conn, addr.IP.String())
	if err != nil {
		t.Errorf("failed upload: %v", err)
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}

	t.Logf("uploaded %s bytes", common.ByteSize(count))
	wg.Wait()
}

//func TestClient_Download(t *testing.T) {
//	const clientIP = "123.124.125.126"
//	var data = []byte("test data")
//
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		if strings.HasSuffix(r.URL.Path, "/download") {
//			w.Header().Set(common.XRequestIPHeader, clientIP)
//			w.WriteHeader(http.StatusOK)
//
//			_, err := w.Write(data)
//
//			if err != nil {
//				t.Errorf("Failed to write data: %v", err)
//			}
//		}
//	}))
//	defer server.Close()
//
//	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 50 * time.Millisecond, Dot: true}
//	client, err := New(params)
//
//	if err != nil {
//		t.Fatalf("failed to create client: %v", err)
//	}
//
//	client.Address = server.URL
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	token, err := auth.ClientToken()
//	if err != nil {
//		t.Fatalf("failed to create token: %v", err)
//	}
//
//	count, ip, err := client.download(ctx, token, server.Client())
//
//	if err != nil {
//		t.Errorf("failed download: %v", err)
//	}
//
//	if ip != clientIP {
//		t.Errorf("want %s, got %s IP address", clientIP, ip)
//	}
//
//	if want := int64(len(data)); count != want {
//		t.Errorf("want %d, got %d", want, count)
//	}
//}
//
//func TestClient_Upload(t *testing.T) {
//	const clientIP = "123.124.125.126"
//
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		if strings.HasSuffix(r.URL.Path, "/upload") {
//			w.Header().Set(common.XRequestIPHeader, clientIP)
//			w.WriteHeader(http.StatusOK)
//
//			buffer := make([]byte, 32)
//
//			n, err := r.Body.Read(buffer)
//			if err != nil {
//				t.Errorf("failed to read body data: %v", err)
//			}
//
//			if n == 0 {
//				t.Error("uploaded zero bytes")
//			}
//
//			if err = r.Body.Close(); err != nil {
//				t.Errorf("failed to close body: %v", err)
//			}
//		}
//	}))
//	defer server.Close()
//
//	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 100 * time.Millisecond, Dot: true}
//	client, err := New(params)
//	if err != nil {
//		t.Fatalf("failed to create client: %v", err)
//	}
//
//	client.Address = server.URL
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	token, err := auth.ClientToken()
//	if err != nil {
//		t.Fatalf("failed to create token: %v", err)
//	}
//
//	count, ip, err := client.upload(ctx, token, server.Client())
//
//	if err != nil {
//		t.Errorf("failed upload: %v", err)
//	}
//
//	if ip != clientIP {
//		t.Errorf("want %s, got %s IP address", clientIP, ip)
//	}
//
//	if count == 0 {
//		t.Error("no uploaded data")
//	}
//}
//
//func TestClient_Start(t *testing.T) {
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		if strings.HasSuffix(r.URL.Path, "/download") {
//			w.WriteHeader(http.StatusOK)
//			_, err := w.Write([]byte{0, 1})
//
//			if err != nil {
//				t.Errorf("Failed to write data: %v", err)
//			}
//		}
//
//		if strings.HasSuffix(r.URL.Path, "/upload") {
//			buffer := make([]byte, 2)
//			if _, err := r.Body.Read(buffer); err != nil {
//				t.Errorf("failed to read body data: %v", err)
//			}
//
//			if err := r.Body.Close(); err != nil {
//				t.Errorf("failed to close body: %v", err)
//			}
//		}
//	}))
//	defer server.Close()
//
//	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 100 * time.Millisecond}
//	client, err := New(params)
//	if err != nil {
//		t.Fatalf("failed to create client: %v", err)
//	}
//
//	client.Address = server.URL
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	var b bytes.Buffer
//	ctx = context.WithValue(ctx, ctxWriterKey, &b)
//
//	err = client.Start(ctx)
//	if err != nil {
//		t.Errorf("failed to start client: %v", err)
//	}
//
//	lines := strings.Split(b.String(), "\n")
//	if n := len(lines); n != 4 {
//		// IP, download, upload, empty line
//		t.Errorf("want 3 lines, got %d: %q", n, lines)
//	}
//
//	if !strings.HasPrefix(lines[0], "IP address:") {
//		t.Error("failed prefix for IP address")
//	}
//
//	if !strings.HasPrefix(lines[1], "Download speed:") {
//		t.Error("failed prefix for download")
//	}
//
//	if !strings.HasPrefix(lines[2], "Upload speed:") {
//		t.Error("failed prefix for upload")
//	}
//
//	client.Dot = false
//	err = client.Start(ctx)
//	if err != nil {
//		t.Errorf("failed to start client: %v", err)
//	}
//}

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

	expected = "address: localhost:8080, timeout: 100ms"
	if got := client.String(); got != expected {
		t.Errorf("want %q, got %q", expected, got)
	}
}

//func TestClient_Token(t *testing.T) {
//	serverTokens := map[uint16]*auth.Token{
//		1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
//		2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
//	}
//	clientToken := &auth.Token{ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}} // client #1
//
//	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
//		if _, err := auth.Authorize(r, serverTokens); err != nil {
//			w.WriteHeader(http.StatusUnauthorized)
//			return
//		}
//
//		buffer := make([]byte, 2)
//		if _, err := r.Body.Read(buffer); err != nil && !errors.Is(err, io.EOF) {
//			t.Errorf("failed to read body data: %v", err)
//		}
//
//		if err := r.Body.Close(); err != nil {
//			t.Errorf("failed to close body: %v", err)
//		}
//
//		w.WriteHeader(http.StatusOK)
//		_, err := w.Write([]byte{0, 1})
//
//		if err != nil {
//			t.Errorf("Failed to write data: %v", err)
//		}
//	}))
//	defer server.Close()
//
//	params := &common.Params{Host: server.URL, Port: 8080, Timeout: 50 * time.Millisecond, Dot: true}
//	client, err := New(params)
//
//	if err != nil {
//		t.Fatalf("failed to create client: %v", err)
//	}
//
//	client.Address = server.URL
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	_, _, err = client.download(ctx, clientToken, server.Client())
//	if err != nil {
//		t.Errorf("failed download: %v", err)
//	}
//
//	_, _, err = client.upload(ctx, clientToken, server.Client())
//	if err != nil {
//		t.Errorf("failed upload: %v", err)
//	}
//
//	clientToken.ClientID = 3 // unknown client
//	_, _, err = client.download(ctx, clientToken, server.Client())
//	if err == nil {
//		t.Errorf("error expected")
//	}
//}
