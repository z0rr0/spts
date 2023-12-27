package server

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

const serverTimeout = 100 * time.Millisecond

func TestNew(t *testing.T) {
	testCases := []struct {
		name      string
		host      string
		port      uint16
		clients   int
		withError bool
	}{
		{name: "valid", host: "localhost", port: 28081, clients: 1},
		{name: "empty_host", port: 28081, clients: 2},
		{name: "not_clients", port: 28081, withError: true},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			params := &common.Params{
				Host:    tc.host,
				Port:    tc.port,
				Timeout: serverTimeout,
				Clients: tc.clients,
			}
			s, err := New(params)

			if (err != nil) != tc.withError {
				t.Errorf("want error %v, got %v", tc.withError, err)
			}

			if err == nil && s == nil {
				t.Error("want server, got nil")
			}
		})
	}
}

type testClient struct {
	id    uint16
	addr  *net.TCPAddr
	token *auth.Token
}

func (c *testClient) connect(download bool) (net.Conn, error) {
	conn, err := net.Dial(c.addr.Network(), c.addr.String())
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	remoteAddr := conn.RemoteAddr().(*net.TCPAddr)
	c.token.IP = remoteAddr.IP
	c.token.Download = download

	if err = c.token.Handshake(conn); err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}

	return conn, nil
}

func (c *testClient) do() error {
	// upload
	conn, err := c.connect(false)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	r := common.NewReader(context.Background())
	_, err = io.Copy(conn, r)

	if err = common.SkipError(err); err != nil {
		return fmt.Errorf("upload read/write: %w", err)
	}

	if err = conn.Close(); err != nil {
		return fmt.Errorf("close upload: %w", err)
	}

	// download
	if conn, err = c.connect(true); err != nil {
		return fmt.Errorf("connect: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), serverTimeout/2)
	defer cancel()

	w := common.NewWriter(ctx)
	_, err = io.Copy(w, conn)

	if err != nil && !errors.Is(err, common.ErrWriterTimeout) {
		return fmt.Errorf("download read/write: %w", err)
	}

	return conn.Close()
}

func tokensToString(tokens map[uint16]*auth.Token) string {
	items := make([]string, 0, len(tokens))

	for clientID, token := range tokens {
		items = append(items, fmt.Sprintf("%d:%s", clientID, hex.EncodeToString(token.Secret)))
	}

	return strings.Join(items, ",")
}

func TestStart(t *testing.T) {
	var (
		params = &common.Params{Host: "127.0.0.1", Port: 28082, Timeout: serverTimeout, Clients: 1}
		tokens = map[uint16]*auth.Token{
			1: {ClientID: 1, Secret: []byte{0x33, 0x12, 0xa1, 0x8b}},
			2: {ClientID: 2, Secret: []byte{0x66, 0x6b, 0xf6, 0xa2}},
		}
		stop = make(chan struct{})
	)

	if err := os.Setenv(auth.ServerEnv, tokensToString(tokens)); err != nil {
		t.Fatalf("failed to set environment variable: %v", err)
	}

	defer func() {
		if err := os.Unsetenv(auth.ServerEnv); err != nil {
			t.Errorf("failed to unset environment variable: %v", err)
		}
	}()

	server, err := New(params)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		<-stop
	}()

	// start server as separate goroutine
	go func() {
		if e := server.Start(ctx); e != nil {
			t.Errorf("server start: %v", e)
		}
		close(stop)
	}()

	client := &testClient{id: 2, addr: &server.addr, token: tokens[2]}

	time.Sleep(2 * time.Second)
	if err = client.do(); err != nil {
		t.Errorf("client do: %v", err)
	}
}
