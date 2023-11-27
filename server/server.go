package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

const (
	shutdownTimeout = 10 * time.Second
	authPrefix      = "Bearer "
)

// ErrResponseFailed is returned when the response failed.
var ErrResponseFailed = errors.New("response failed")

type handlerType func(w http.ResponseWriter, r *http.Request) error

// Server is a server data.
type Server struct {
	address string
	timeout time.Duration
}

// New creates a new server.
func New(host string, port uint64, timeout time.Duration) (*Server, error) {
	address, err := common.Address(host, port)
	if err != nil {
		return nil, err
	}

	return &Server{address: address, timeout: timeout}, nil
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	srv := s.createHandlers()
	connectionsClosed := make(chan struct{})

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server Shutdown error", "error", err)
		}
		close(connectionsClosed)
	}()

	slog.Info("HTTP server starting", "PID", os.Getpid(), "address", s.address, "timeout", s.timeout)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server ListenAndServe error", "error", err)
	}

	<-connectionsClosed
	slog.Info("HTTP server stopped")
	return nil
}

func (s *Server) createHandlers() *http.Server {
	srv := &http.Server{
		Addr:           s.address,
		Handler:        http.DefaultServeMux,
		ReadTimeout:    s.timeout * 2,
		WriteTimeout:   s.timeout * 2,
		MaxHeaderBytes: common.KB,
	}

	tokens := auth.LoadTokens()
	handlers := map[string]handlerType{
		common.UploadURL:   s.upload,
		common.DownloadURL: s.download,
	}

	http.HandleFunc("/", rootHandler(tokens, handlers))
	return srv
}

// download writes data to client.
func (s *Server) download(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=download.log")

	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	reader := common.NewReader(ctx)
	count, buffer := 0, make([]byte, common.DefaultBufSize)

	defer func() {
		slog.Debug("writes", "count", common.ByteSize(count))
	}()

	for {
		n, err := reader.Read(buffer)

		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return errors.Join(ErrResponseFailed, fmt.Errorf("read: %w", err))
		}

		_, err = w.Write(buffer[:n])
		if err != nil {
			return errors.Join(ErrResponseFailed, fmt.Errorf("write: %w", err))
		}

		count += n
	}
}

// upload reads data from client.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	count, err := common.Read(ctx, r.Body, common.DefaultBufSize)
	if err != nil {
		return errors.Join(ErrResponseFailed, fmt.Errorf("read: %w", err))
	}

	err = r.Body.Close()
	if err != nil {
		return errors.Join(ErrResponseFailed, fmt.Errorf("close body: %w", err))
	}

	countSize := common.ByteSize(count)
	msg := fmt.Sprintf("read %s bytes\n", countSize)
	_, err = w.Write([]byte(msg))

	if err != nil {
		return errors.Join(ErrResponseFailed, fmt.Errorf("write: %w", err))
	}

	slog.Debug("reads", "count", countSize)
	return nil
}

func rootHandler(tokens map[string]struct{}, handlers map[string]handlerType) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			start = time.Now()
			code  = http.StatusOK
			url   = strings.TrimRight(r.URL.Path, "/ ")
		)
		slog.Info("request received", "method", r.Method, "url", url, "remoteAddr", r.RemoteAddr)

		defer func() {
			if code != http.StatusOK {
				http.Error(w, http.StatusText(code), code)
			}

			slog.Info(
				"request done",
				"method", r.Method, "code", code, "duration", time.Since(start), "remoteAddr", r.RemoteAddr,
			)
		}()

		if !auth.Authorize(tokens, r) {
			code = http.StatusUnauthorized
			return
		}

		handler, ok := handlers[url]
		if !ok {
			code = http.StatusNotFound
			return
		}

		err := handler(w, r)
		if err != nil {
			slog.Error("request", "error", err)
			if strings.Contains(err.Error(), "http: request body too large") {
				code = http.StatusRequestEntityTooLarge
			} else {
				code = http.StatusInternalServerError
			}
			return
		}
	}
}
