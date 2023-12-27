package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

const (
	testEnv        = "1:3312a18b"
	testAccTimeout = 20 * time.Millisecond
)

var (
	outRe = regexp.MustCompile(`^IP address:\s{5}.*\nDownload speed: .*\nUpload speed:\s{3}.*\n$`)
)

type testServer struct {
	listener net.Listener
	stop     chan struct{}
	wait     chan struct{}
}

func (t *testServer) Stop() {
	close(t.stop)
	<-t.wait
}

func createServer(t *testing.T, f func(conn net.Conn) error) (*testServer, error) {
	var opErr *net.OpError
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})

	if err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	wait := make(chan struct{})
	srv := &testServer{listener: listener, stop: stop, wait: wait}

	go func() {
		defer func() {
			if e := listener.Close(); e != nil {
				t.Errorf("failed to close listener: %v", e)
			}
			close(wait)
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
					if !(errors.As(e, &opErr) && opErr.Timeout()) {
						t.Errorf("failed to accept connection: %v", e)
					}
					continue
				}

				if e = conn.SetDeadline(time.Now().Add(testAccTimeout * 5)); e != nil {
					t.Errorf("failed to set deadline: %v", e)
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

	return srv, nil
}

func TestNew(t *testing.T) {
	testCases := []struct {
		name      string
		host      string
		port      uint16
		client    string
		errSubstr string
	}{
		{name: "valid", host: "localhost", port: 28082, client: "address: localhost:28082, timeout: 20ms"},
		{name: "invalid_port", host: "localhost", errSubstr: "invalid port"},
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
	const size = int64(128 * common.KB)

	srv, err := createServer(t, func(conn net.Conn) error {
		data := make([]byte, size)
		buffer := bytes.NewReader(data)

		n, err := io.Copy(conn, buffer)

		if err != nil {
			return err
		}

		if n != size {
			return fmt.Errorf("failed to write buffer: %d != %d", n, size)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	addr, ok := srv.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("failed to get listener address")
	}

	client := Client{Params: common.Params{Host: addr.IP.String(), Port: uint16(addr.Port)}}
	conn, err := net.Dial(addr.Network(), addr.String())

	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	count, err := client.download(ctx, conn, addr.IP.String())
	if err != nil {
		t.Errorf("failed download: %v", err)
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}

	if count != uint64(size) {
		t.Errorf("want %d, got %d", size, count)
	}

}

func TestClient_Upload(t *testing.T) {
	var (
		total   uint64
		stopped = make(chan struct{})
	)

	srv, err := createServer(t, func(conn net.Conn) error {
		defer close(stopped)
		n, err := io.Copy(io.Discard, conn)

		if err != nil {
			return err
		}

		total = uint64(n)
		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	addr, ok := srv.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("failed to get listener address")
	}

	client := Client{Params: common.Params{Host: addr.IP.String(), Port: uint16(addr.Port)}}
	conn, err := net.Dial(addr.Network(), addr.String())

	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testAccTimeout)
	defer cancel()

	count, err := client.upload(ctx, conn, addr.IP.String())
	if err != nil {
		t.Errorf("failed upload: %v", err)
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}

	<-stopped
	if count != total {
		t.Errorf("want %d, got %d", total, count)
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

	expected = "address: localhost:8080, timeout: 100ms"
	if got := client.String(); got != expected {
		t.Errorf("want %q, got %q", expected, got)
	}
}

func TestClient_Handshake(t *testing.T) {
	var tokens = map[uint16]*auth.Token{
		1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
		2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
	}

	srv, err := createServer(t, func(conn net.Conn) error {
		// read handshake
		token, err := auth.Verify(conn, tokens)
		if err != nil {
			return err
		}

		remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			return common.ErrIPAddress
		}
		token.IP = remoteAddr.IP

		// write handshake reply
		header := token.Sign()
		if _, err = conn.Write(header); err != nil {
			return fmt.Errorf("write header: %w", err)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	addr, ok := srv.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("failed to get listener address")
	}

	client := Client{Params: common.Params{Host: addr.IP.String(), Port: uint16(addr.Port)}}
	conn, err := net.Dial(addr.Network(), addr.String())

	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	clientID, _, err := client.handshake(conn, tokens[1], true)

	if err != nil {
		t.Fatalf("failed handshake: %v", err)
	}

	if clientID != 1 {
		t.Errorf("want %d, got %d", 1, clientID)
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}

	// unknown token
	if conn, err = net.Dial(addr.Network(), addr.String()); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	token := &auth.Token{ClientID: 3, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}}
	_, _, err = client.handshake(conn, token, true)

	if err == nil {
		t.Errorf("error expected")
	}

	if err = conn.Close(); err != nil {
		t.Errorf("failed to close connection: %v", err)
	}
}

func TestClient_Start(t *testing.T) {
	var (
		stopped = make(chan struct{})
		tokens  = map[uint16]*auth.Token{
			1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
		}
	)

	srv, err := createServer(t, func(conn net.Conn) error {
		// read handshake
		token, err := auth.Verify(conn, tokens)
		if err != nil {
			return err
		}

		remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
		if !ok {
			return common.ErrIPAddress
		}
		token.IP = remoteAddr.IP

		// write handshake reply
		header := token.Sign()
		if _, err = conn.Write(header); err != nil {
			return fmt.Errorf("write header: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), testAccTimeout/2)
		defer cancel()

		if token.Download {
			r := common.NewReader(ctx)
			n, e := io.Copy(conn, r)

			if e = common.SkipError(e); e != nil {
				return e
			}
			t.Logf("downloaded %d bytes", n)
		} else {
			defer close(stopped)

			w := common.NewWriter(ctx)
			n, e := io.Copy(w, conn)

			if e != nil && !errors.Is(e, common.ErrWriterTimeout) {
				return e
			}
			t.Logf("uploaded %d bytes", n)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	defer srv.Stop()

	addr, ok := srv.listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("failed to get listener address")
	}

	var expectedWriter = &bytes.Buffer{}
	ctx := context.WithValue(context.Background(), ctxWriterKey, expectedWriter)

	client := Client{
		Params: common.Params{
			Host:    addr.IP.String(),
			Port:    uint16(addr.Port),
			Timeout: testAccTimeout / 2,
		},
	}
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	if err = os.Setenv(auth.ClientEnv, testEnv); err != nil {
		t.Fatalf("failed to set environment variable: %v", err)
	}

	defer func() {
		if err = os.Unsetenv(auth.ClientEnv); err != nil {
			t.Errorf("failed to unset environment variable: %v", err)
		}
	}()

	if err = client.Start(ctx); err != nil {
		t.Fatalf("failed to start client: %v", err)
	}

	<-stopped
	if s := expectedWriter.String(); !outRe.MatchString(s) {
		t.Errorf("want %q, got %q", outRe.String(), s)
	}
}
