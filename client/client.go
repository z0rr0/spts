package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/z0rr0/spts/auth0"
	"github.com/z0rr0/spts/auth0/token"
	"github.com/z0rr0/spts/common"
)

// ErrConnectionFailed is returned when the connection failed.
var ErrConnectionFailed = errors.New("connection failed")

// Client is a client data.
type Client struct {
	common.Params
}

// New creates a new client.
func New(params *common.Params) (*Client, error) {
	if params.Port < 1 {
		return nil, errors.Join(common.ErrInvalidPort, errors.New("port number must be greater than 0"))
	}

	if params.Host == "" {
		return nil, errors.New("host address is empty")
	}

	return &Client{Params: *params}, nil
}

// String implements Stringer interface.
func (c *Client) String() string {
	return fmt.Sprintf("address: %s, timeout: %s", c.Address(), c.Timeout)
}

// Start does a client request.
func (c *Client) Start(ctx context.Context) error {
	var (
		newLine  = c.NewLine()
		pgWriter = progressWriter(ctx)
	)

	t, err := auth0.ClientToken()
	if err != nil {
		return err
	}
	slog.Debug("token", "client", t.ClientID)

	t.Download = true
	speed, ip, err := c.run(ctx, pgWriter, t)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(pgWriter, "%sIP address:     %s\n", newLine, ip)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(pgWriter, "%sDownload speed: %s\n", newLine, speed)
	if err != nil {
		return err
	}

	t.Download = false
	speed, _, err = c.run(ctx, pgWriter, t)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(pgWriter, "%sUpload speed:   %s\n", newLine, speed)
	return err
}

// Upload does a client requests.
func (c *Client) run(ctx context.Context, pgWriter io.Writer, t *token.Token) (string, string, error) {
	var (
		dialer  net.Dialer
		count   uint64
		timeout = c.Timeout
	)

	if c.Params.Dot {
		prg := newProgress(pgWriter, time.Second)
		defer prg.done()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := dialer.DialContext(ctx, "tcp", c.Address())
	if err != nil {
		return "", "", errors.Join(ErrConnectionFailed, fmt.Errorf("dial: %w", err))
	}

	defer func() {
		if e := conn.Close(); e != nil {
			slog.Error("connection", "close_error", e)
		}
	}()

	if err = c.handshake(conn, t); err != nil {
		return "", "", err
	}

	slog.Debug(
		"connection",
		"address", conn.RemoteAddr().String(), "client", t.ClientID, "action", t.Action(), "timeout", timeout,
	)
	start := time.Now()

	if t.Download {
		count, err = c.download(ctx, conn)
	} else {
		count, err = c.upload(ctx, conn)
	}

	if err != nil {
		return "", "", err
	}

	ip := t.IP.String()
	slog.Debug("connection", "action", t.Action(), "ip", ip, "count", common.ByteSize(count))

	return common.Speed(time.Since(start), count, common.SpeedSeconds), ip, nil
}

// handshake does a client handshake, sends token and receives one back.
func (c *Client) handshake(conn net.Conn, t *token.Token) error {
	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return common.ErrIPAddress
	}

	ip := remoteAddr.IP
	if t == nil {
		return nil // no token, no handshake
	}

	t.IP = ip
	return t.Handshake(conn)
}

// download gets data from server.
func (c *Client) download(ctx context.Context, conn io.Reader) (uint64, error) {
	w := common.NewWriter(ctx)
	n, err := io.Copy(w, conn) // successful Copy returns err == nil, not err == io.EOF

	if err != nil && !errors.Is(err, common.ErrWriterTimeout) {
		return 0, errors.Join(ErrConnectionFailed, fmt.Errorf("download read/write: %w", err))
	}

	return uint64(n), nil
}

// upload sends data to server.
func (c *Client) upload(ctx context.Context, conn io.Writer) (uint64, error) {
	r := common.NewReader(ctx)
	n, err := io.Copy(conn, r)

	if err = common.SkipError(err); err != nil {
		return 0, errors.Join(ErrConnectionFailed, fmt.Errorf("upload read/write: %w", err))
	}

	return uint64(n), nil
}
