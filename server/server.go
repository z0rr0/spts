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
	acceptAddTime = 100 * time.Millisecond
)

var (
	ErrSkipConnection = errors.New("skip connection")
	ErrAcceptTimeout  = errors.New("accept timeout")
	ErrDataWriteRead  = errors.New("data write/read")
)

// Server is a server data.
type Server struct {
	common.Params
	addr net.TCPAddr
}

// New creates a new server.
func New(params *common.Params) (*Server, error) {
	if params.Clients < 1 {
		return nil, errors.New("allow clients number must be greater than 0")
	}

	addr := net.TCPAddr{IP: net.ParseIP(params.Host), Port: int(params.Port)}
	return &Server{Params: *params, addr: addr}, nil
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	slog.Info("server starting", "PID", os.Getpid(), "address", s.Address(), "timeout", s.Timeout)
	defer slog.Info("server stopped")

	if err := s.ListenAndServe(ctx); err != nil {
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
			// error can be from context or listener
			if err = ctx.Err(); err != nil {
				return nil, fmt.Errorf("listener accept context error: %w", err)
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
				// after timeout, context cancellation
				if errors.Is(err, context.Canceled) {
					slog.Info("listener", "context", err)
				} else {
					slog.Error("listener", "error", err)
				}
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
		wg.Add(1)
		go func(c net.Conn) {
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

	// write handshake reply,
	// auth.Verify already updated temporary token's parts
	header := token.Sign()
	if _, err = conn.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	slog.Info("connection", "address", remoteAddr.String(), "client", token.ClientID, "action", token.Action())

	if token.Download {
		err = download(ctx, conn)
	} else {
		err = upload(ctx, conn)
	}

	return err
}

func download(ctx context.Context, w io.Writer) error {
	r := common.NewReader(ctx)
	n, err := io.Copy(w, r)

	if err = common.SkipError(err); err != nil {
		return errors.Join(ErrDataWriteRead, fmt.Errorf("download copy: %w", err))
	}

	slog.Info("writes", "count", common.ByteSize(uint64(n)))
	return nil
}

func upload(ctx context.Context, r io.Reader) error {
	w := common.NewWriter(ctx)
	n, err := io.Copy(w, r)

	if err != nil && !errors.Is(err, common.ErrWriterTimeout) {
		return errors.Join(ErrDataWriteRead, fmt.Errorf("upload copy: %w", err))
	}

	slog.Info("reads", "count", common.ByteSize(uint64(n)))
	return nil
}
