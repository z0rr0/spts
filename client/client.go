package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

type ctxType string

const ctxWriterKey ctxType = "progressWriter"

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

	token, err := auth.ClientToken()
	if err != nil {
		return err
	}

	slog.Debug("token", "client", token.ClientID)

	speed, ip, err := c.run(ctx, pgWriter, token, true)
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

	speed, _, err = c.run(ctx, pgWriter, token, false)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(pgWriter, "%sUpload speed:   %s\n", newLine, speed)
	return err
}

// Upload does a client POST request with body.
func (c *Client) run(ctx context.Context, pgWriter io.Writer, token *auth.Token, download bool) (string, string, error) {
	var (
		dialer  net.Dialer
		count   uint64
		timeout = c.Timeout
	)

	if c.Params.Dot {
		prg := newProgress(pgWriter, time.Second)
		defer prg.done()
	}

	if download {
		timeout *= common.TimeoutMultiplier
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

	client, ip, err := c.handshake(conn, token, download)
	if err != nil {
		return "", "", err
	}

	slog.Debug(
		"connection",
		"address", conn.RemoteAddr().String(), "client", client, "download", download, "timeout", timeout,
	)
	start := time.Now()

	if download {
		count, err = c.download(ctx, conn)
	} else {
		count, err = c.upload(ctx, conn)
	}

	if err != nil {
		return "", "", err
	}

	slog.Debug("connection", "download", download, "ip", ip, "count", common.ByteSize(count))
	return common.Speed(time.Since(start), count, common.SpeedSeconds), ip, nil
}

// handshake does a client handshake, sends token and receives one back.
func (c *Client) handshake(conn net.Conn, token *auth.Token, download bool) (uint16, string, error) {
	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return 0, "", common.ErrIPAddress
	}

	ip := remoteAddr.IP
	if token == nil {
		return 0, ip.String(), nil // no token, no handshake
	}

	token.IP = ip
	token.Download = download

	if err := token.Handshake(conn); err != nil {
		return 0, "", err
	}

	return token.ClientID, ip.String(), nil
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
