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

	"github.com/z0rr0/spts/auth0"
	"github.com/z0rr0/spts/auth0/token"
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

// Server is a server struct.
type Server struct {
	common.Params
	addr   net.TCPAddr
	tokens map[uint16]*token.Token
}

type acceptedConn struct {
	conn net.Conn
	err  error
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

// ListenAndServe listens and serves incoming connections.
func (s *Server) ListenAndServe(ctx context.Context) error {
	tokens, err := auth0.ServerTokens(s.Chunk)
	if err != nil {
		return err
	}

	s.tokens = tokens
	slog.Info("tokens", "count", len(s.tokens))

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

	for ac := range s.connChan(ctx, listener, semaphore) {
		wg.Add(1)
		go s.handleConnection(ctx, &wg, ac, semaphore)
	}

	<-semaphore
	wg.Wait()

	return nil
}

func (s *Server) connChan(ctx context.Context, listener *net.TCPListener, semaphore chan struct{}) chan acceptedConn {
	ch := make(chan acceptedConn)

	go func() {
		defer close(ch)

		for {
			ac := s.connAccept(ctx, listener, semaphore)

			switch {
			case errors.Is(ac.err, ErrAcceptTimeout):
				slog.Debug("listener", "accept_timeout", ac.err, "timeout", acceptTimeout)
			case errors.Is(ac.err, ErrSkipConnection):
				slog.Info("listener", "skip_error", ac.err)
			case ac.err != nil:
				// after timeout, context cancellation
				if errors.Is(ac.err, context.Canceled) {
					slog.Info("listener", "context", ac.err)
				} else {
					slog.Error("listener", "error", ac.err)
				}
				return
			default:
				slog.Info("listener", "accepted", ac.conn.RemoteAddr())
			}

			ch <- ac
		}
	}()

	return ch
}

func (s *Server) connAccept(ctx context.Context, listener *net.TCPListener, semaphore chan struct{}) acceptedConn {
	var (
		conn        *net.TCPConn
		opErr       *net.OpError
		err         error
		addDuration = s.Timeout + acceptAddTime
	)
	// set limit for AcceptTCP timeout,
	// it's only to prevent blocking and periodically check context cancellation
	if err = listener.SetDeadline(time.Now().Add(acceptTimeout)); err != nil {
		return acceptedConn{err: errors.Join(ErrSkipConnection, fmt.Errorf("listener deadline: %w", err))}
	}

	semaphore <- struct{}{}
	conn, err = listener.AcceptTCP() // blocking call, wait for new client's connection

	if err != nil {
		if errors.As(err, &opErr) && opErr.Timeout() {
			// error can be from context or listener
			if err = ctx.Err(); err != nil {
				return acceptedConn{err: fmt.Errorf("listener accept context error: %w", err)}
			}
			return acceptedConn{err: ErrAcceptTimeout}
		}

		return acceptedConn{err: errors.Join(ErrSkipConnection, fmt.Errorf("listener accept: %w", err))}
	}

	if err = connSetDeadline(conn, addDuration, common.TimeoutMultiplier); err != nil {
		err = errors.Join(ErrSkipConnection, fmt.Errorf("connection deadline: %w", err))

		// connection was successfully accepted, but deadline failed, so close it and stop handling
		if e := conn.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("connection close: %w", e))
		}

		return acceptedConn{err: err}
	}

	return acceptedConn{conn: conn}
}

func (s *Server) handleConnection(ctx context.Context, wg *sync.WaitGroup, c acceptedConn, semaphore chan struct{}) {
	defer func() {
		<-semaphore
		wg.Done()
	}()

	if c.err != nil {
		slog.Error("connection", "accept_error", c.err)
		return
	}

	ctxConn, cancel := context.WithTimeout(ctx, s.Timeout)
	if e := handleConnection(ctxConn, c.conn, s.tokens); e != nil {
		slog.Error("connection", "handling_error", e)
	}

	cancel()
}

func handleConnection(ctx context.Context, conn net.Conn, tokens map[uint16]*token.Token) error {
	defer func() {
		if e := conn.Close(); e != nil {
			slog.Error("connection", "close_error", e)
		}
	}()

	// read handshake
	t, err := token.Verify(conn, tokens)
	if err != nil {
		return err
	}

	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)

	if !ok {
		return common.ErrIPAddress
	}
	t.IP = remoteAddr.IP

	// write handshake reply,
	// auth0.Verify already updated temporary token's parts
	header, err := t.Sign()
	if err != nil {
		return err // unexpected error
	}

	if _, err = conn.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	slog.Info("connection", "address", remoteAddr.String(), "client", t.ClientID, "action", t.Action())

	if t.Download {
		err = download(ctx, conn)
	} else {
		err = upload(ctx, conn)
	}

	return err
}

// download writes data to connection.
// It uses short context timeout.
func download(ctx context.Context, w io.Writer) error {
	r := common.NewReader(ctx)
	n, err := io.Copy(w, r)

	if err = common.SkipError(err); err != nil {
		return errors.Join(ErrDataWriteRead, fmt.Errorf("download copy: %w", err))
	}

	slog.Info("writes", "count", common.ByteSize(uint64(n)))
	return nil
}

// upload reads data from connection.
// It's needed longer context timeout due to network latency.
func upload(ctx context.Context, r io.Reader) error {
	w := common.NewWriter(ctx)
	n, err := io.Copy(w, r)

	if err != nil && !errors.Is(err, common.ErrWriterTimeout) {
		return errors.Join(ErrDataWriteRead, fmt.Errorf("upload copy: %w", err))
	}

	slog.Info("reads", "count", common.ByteSize(uint64(n)))
	return nil
}

// connSetDeadline sets a deadline for a connection.
func connSetDeadline(conn net.Conn, timeout time.Duration, multiplier time.Duration) error {
	if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}

	timeout *= multiplier
	return conn.SetReadDeadline(time.Now().Add(timeout))
}
