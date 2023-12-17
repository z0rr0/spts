package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

type ctxType string

const ctxWriterKey ctxType = "writer"

// ErrConnectionFailed is returned when the connection failed.
var ErrConnectionFailed = errors.New("connection failed")

// Client is a client data.
type Client struct {
	common.ServiceBase
}

// New creates a new client.
func New(params *common.Params) (*Client, error) {
	address, err := common.Address(params.Host, params.Port)
	if err != nil {
		return nil, err
	}

	return &Client{ServiceBase: common.ServiceBase{Address: address, Params: params}}, nil
}

// String implements Stringer interface.
func (c *Client) String() string {
	return fmt.Sprintf("address: %s, timeout: %s", c.Address, c.Timeout)
}

func (c *Client) writer(ctx context.Context) io.Writer {
	var writer io.Writer = os.Stdout

	if ctxWriter, ok := ctx.Value(ctxWriterKey).(io.Writer); ok {
		writer = ctxWriter
	}

	return writer
}

// Start does a client request.
func (c *Client) Start(ctx context.Context) error {
	var (
		newLine = c.NewLine()
		writer  = c.writer(ctx)
	)

	token, err := auth.ClientToken()
	if err != nil {
		return err
	}

	slog.Debug("token", "client", token.ClientID)

	speed, ip, err := c.run(ctx, writer, token, true)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(writer, "%sIP address:     %s\n", newLine, ip)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(writer, "%sDownload speed: %s\n", newLine, speed)
	if err != nil {
		return err
	}

	speed, _, err = c.run(ctx, writer, token, false)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(writer, "%sUpload speed:   %s\n", newLine, speed)
	return err
}

// Upload does a client POST request with body.
func (c *Client) run(ctx context.Context, w io.Writer, token *auth.Token, download bool) (string, string, error) {
	var (
		d     net.Dialer
		count uint64
	)

	if c.Params.Dot {
		prg := newProgress(w, time.Second)
		defer prg.done()
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	conn, err := d.DialContext(ctx, "tcp", c.Address)
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

	slog.Debug("connection", "address", conn.RemoteAddr().String(), "client", client, "download", download)
	start := time.Now()

	if download {
		count, err = c.download(ctx, conn, ip)
	} else {
		count, err = c.upload(ctx, conn, ip)
	}

	if err != nil {
		return "", "", err
	}

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
func (c *Client) download(ctx context.Context, conn net.Conn, ip string) (uint64, error) {
	count, err := common.Read(ctx, conn, common.DefaultBufSize)
	if err != nil {
		return 0, errors.Join(ErrConnectionFailed, fmt.Errorf("read: %w", err))
	}

	slog.Debug("connection", "action", "download", "ip", ip, "count", common.ByteSize(count))
	return count, nil
}

// upload sends data to server.
func (c *Client) upload(ctx context.Context, conn net.Conn, ip string) (uint64, error) {
	reader := common.NewReader(ctx)
	_, err := io.Copy(conn, reader)

	if err = skipError(err); err != nil {
		return 0, errors.Join(ErrConnectionFailed, fmt.Errorf("upload read/write: %w", err))
	}

	count := reader.Count.Load()

	slog.Debug("connection", "action", "upload", "ip", ip, "count", common.ByteSize(count))
	return count, nil
}

func skipError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) {
		return nil
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err != nil && strings.Contains(opErr.Err.Error(), "broken pipe") {
			return nil
		}
	}

	return err
}
