package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

const (
	acceptTimeout = 2 * time.Second
	acceptAddTime = 50 * time.Millisecond
)

var (
	ErrSkipConnection = errors.New("skip connection")
	ErrAcceptTimeout  = errors.New("accept timeout")
	ErrDataWriteRead  = errors.New("data write/read")
)

// Server is a server data.
type Server struct {
	common.ServiceBase
	addr net.TCPAddr
}

// New creates a new server.
func New(params *common.Params) (*Server, error) {
	address, err := common.Address(params.Host, params.Port)
	if err != nil {
		return nil, err
	}

	if params.Clients < 1 {
		return nil, errors.New("allow clients number must be greater than 0")
	}

	addr := net.TCPAddr{IP: net.ParseIP(params.Host), Port: int(params.Port)}
	return &Server{ServiceBase: common.ServiceBase{Address: address, Params: params}, addr: addr}, nil
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("server starting", "PID", os.Getpid(), "address", s.Address, "timeout", s.Timeout)
	defer slog.Info("server stopped")

	err := s.ListenAndServe(ctx)
	if err != nil {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
}

func (s *Server) connAccept(ctx context.Context, listener *net.TCPListener, semaphore chan struct{}) (net.Conn, error) {
	var (
		err           error
		conn          *net.TCPConn
		opErr         *net.OpError
		freeSemaphore bool
		addDuration   = s.Timeout + acceptAddTime
	)

	defer func() {
		if freeSemaphore {
			<-semaphore
		}
	}()

	// set limit for AcceptTCP timeout,
	// it's only to prevent blocking and periodically check context cancellation
	if err = listener.SetDeadline(time.Now().Add(acceptTimeout)); err != nil {
		return nil, errors.Join(ErrSkipConnection, fmt.Errorf("listener deadline: %w", err))
	}

	semaphore <- struct{}{}
	freeSemaphore = true
	conn, err = listener.AcceptTCP()

	if err != nil {
		if errors.As(err, &opErr) && opErr.Timeout() {
			if err = ctx.Err(); err != nil {
				return nil, fmt.Errorf("context error: %w", err)
			}
			return nil, ErrAcceptTimeout
		}

		return nil, errors.Join(ErrSkipConnection, fmt.Errorf("listener accept: %w", err))
	}

	if err = conn.SetDeadline(time.Now().Add(addDuration)); err != nil {
		err = errors.Join(ErrSkipConnection, fmt.Errorf("connection deadline: %w", err))

		// connection was successfully accepted, but deadline failed, so close it and stop handling
		if e := conn.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("connection close: %w", e))
		}

		return nil, err
	}

	// no errors, don't need to release semaphore
	// it will be released in handleConnection
	freeSemaphore = false
	return conn, nil
}

func (s *Server) connChan(ctx context.Context, listener *net.TCPListener, semaphore chan struct{}) chan net.Conn {
	ch := make(chan net.Conn)

	go func() {
		for {
			conn, err := s.connAccept(ctx, listener, semaphore)

			switch {
			case errors.Is(err, ErrAcceptTimeout):
				slog.Debug("listener", "accept_timeout", err, "timeout", acceptTimeout)
			case errors.Is(err, ErrSkipConnection):
				slog.Info("listener", "skip_error", err)
			case err != nil:
				// after timeout and context cancellation
				slog.Error("listener", "error", err)
				close(ch)
				return
			default:
				slog.Info("listener", "accepted", conn.RemoteAddr())
				ch <- conn
			}
		}
	}()

	return ch
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	tokens, err := auth.ServerTokens()
	if err != nil {
		return err
	}

	slog.Info("tokens", "count", len(tokens))

	listener, err := net.ListenTCP("tcp", &s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	defer func() {
		if e := listener.Close(); e != nil {
			slog.Error("listener", "close_error", e)
		}
	}()

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, s.Clients) // limit concurrent requests
	defer close(semaphore)

	for conn := range s.connChan(ctx, listener, semaphore) {
		go func(c net.Conn) {
			wg.Add(1)
			ctxConn, cancel := context.WithTimeout(ctx, s.Timeout)

			if e := handleConnection(ctxConn, c, tokens); e != nil {
				slog.Error("connection", "handling_error", e)
			}

			cancel()
			<-semaphore // release semaphore for next request
			wg.Done()
		}(conn)
	}

	wg.Wait()
	return nil
}

func handleConnection(ctx context.Context, conn net.Conn, tokens map[uint16]*auth.Token) error {
	defer func() {
		if e := conn.Close(); e != nil {
			slog.Error("connection", "close_error", e)
		}
	}()

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

	header := token.Sign()
	if _, err = conn.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	slog.Info("connection", "address", remoteAddr.String(), "client", token.ClientID, "download", token.Download)

	if token.Download {
		err = download(ctx, conn)
	} else {
		err = upload(ctx, conn)
	}

	return err
}

func download(ctx context.Context, w io.Writer) error {
	var count uint64

	reader := common.NewReader(ctx)
	buffer := make([]byte, common.DefaultBufSize)

	for {
		n, err := reader.Read(buffer)

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return errors.Join(ErrDataWriteRead, fmt.Errorf("download read: %w", err))
		}

		_, err = w.Write(buffer[:n])
		if err != nil {
			return errors.Join(ErrDataWriteRead, fmt.Errorf("download write: %w", err))
		}

		count += uint64(n)
	}

	slog.Info("writes", "count", common.ByteSize(count))
	return nil
}

func upload(ctx context.Context, r io.Reader) error {
	count, err := common.Read(ctx, r, common.DefaultBufSize)
	if err != nil {
		return errors.Join(ErrDataWriteRead, fmt.Errorf("upload read: %w", err))
	}

	slog.Info("reads", "count", common.ByteSize(count))
	return nil
}
