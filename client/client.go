package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/z0rr0/spts/auth"
	"github.com/z0rr0/spts/common"
)

type ctxType string

const ctxWriterKey ctxType = "writer"

// ErrRequestFailed is returned when the request failed.
var ErrRequestFailed = errors.New("request failed")

type handlerType func(context.Context, string, *http.Client) (int64, string, error)

// Client is a client data.
type Client struct {
	common.ServiceBase
}

// New creates a new client.
func New(params *common.Params) (*Client, error) {
	address, err := common.URL(params.Host, params.Port)
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
		token   = auth.Token()
	)
	slog.Debug("token", "length", len(token))

	tr := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		WriteBufferSize: common.DefaultBufSize,
		ReadBufferSize:  common.DefaultBufSize,
	}
	client := &http.Client{Transport: tr}

	speed, ip, err := c.run(ctx, client, writer, token, c.download)
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

	speed, _, err = c.run(ctx, client, writer, token, c.upload)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(writer, "%sUpload speed:   %s\n", newLine, speed)
	return err
}

// Upload does a client POST request with body.
func (c *Client) run(ctx context.Context, client *http.Client, w io.Writer, token string, handler handlerType) (string, string, error) {
	if c.Params.Dot {
		prg := newProgress(w, time.Second)
		defer prg.done()
	}

	start := time.Now()
	count, ip, err := handler(ctx, token, client)

	if err != nil {
		return "", "", err
	}

	return common.Speed(time.Since(start), count, common.SpeedSeconds), ip, nil
}

// download gets data from server.
func (c *Client) download(ctx context.Context, token string, client *http.Client) (int64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	requestURL := c.Address + common.DownloadURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)

	if err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("create: %w", err))
	}

	if token != "" {
		req.Header.Set(auth.AuthorizationHeader, auth.Prefix+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("do: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("status: %d", resp.StatusCode))
	}

	ip := resp.Header.Get(common.XRequestIPHeader)
	count, err := common.Read(ctx, resp.Body, common.DefaultBufSize)
	if err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("read: %w", err))
	}

	slog.Debug("reads", "ip", ip, "count", common.ByteSize(count))
	return int64(count), ip, resp.Body.Close()
}

// upload sends data to server.
func (c *Client) upload(ctx context.Context, token string, client *http.Client) (int64, string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	body := common.NewReader(ctx)
	requestURL := c.Address + common.UploadURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, body)

	if err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("create: %w", err))
	}

	if token != "" {
		req.Header.Set(auth.AuthorizationHeader, auth.Prefix+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("do: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("status: %d", resp.StatusCode))
	}

	if err = resp.Body.Close(); err != nil {
		return 0, "", errors.Join(ErrRequestFailed, fmt.Errorf("close body: %w", err))
	}

	count := body.Count.Load()
	ip := resp.Header.Get(common.XRequestIPHeader)
	slog.Debug("writes", "ip", ip, "count", common.ByteSize(int(count)))

	return count, ip, nil
}
