package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/z0rr0/spts/common"
)

const insecurePrefix = "http://"

// ErrRequestFailed is returned when the request failed.
var ErrRequestFailed = errors.New("request failed")

type handlerType func(context.Context, *http.Client) (int64, error)

// Client is a client data.
type Client struct {
	address string
	timeout time.Duration
	noDot   bool
}

// New creates a new client.
func New(host string, port uint64, timeout time.Duration, noDot bool) (*Client, error) {
	address, err := common.Address(host, port)
	if err != nil {
		return nil, err
	}

	if !(strings.HasPrefix(address, insecurePrefix) || strings.HasPrefix(address, "https://")) {
		address = insecurePrefix + address
	}

	return &Client{address: strings.TrimRight(address, "/ "), timeout: timeout, noDot: noDot}, nil
}

// String implements Stringer interface.
func (c *Client) String() string {
	return fmt.Sprintf("address: %s, timeout: %s", c.address, c.timeout)
}

// Start does a client request.
func (c *Client) Start(ctx context.Context) error {
	var newLine = "\n"
	tr := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxConnsPerHost: 1}
	client := &http.Client{Transport: tr}

	speed, err := c.run(ctx, client, c.download)
	if err != nil {
		return err
	}

	if c.noDot {
		newLine = ""
	}

	fmt.Printf("%sDownload speed: %s\n", newLine, speed)

	speed, err = c.run(ctx, client, c.upload)
	if err != nil {
		return err
	}

	fmt.Printf("%sUpload speed:   %s\n", newLine, speed)

	return nil
}

// Upload does a client POST request with body.
func (c *Client) run(ctx context.Context, client *http.Client, handler handlerType) (string, error) {
	start := time.Now()

	if !c.noDot {
		progressCh := progress(time.Second)
		defer close(progressCh)
	}

	count, err := handler(ctx, client)
	if err != nil {
		return "", err
	}

	return common.Speed(start, count), nil
}

// download gets data from server.
func (c *Client) download(ctx context.Context, client *http.Client) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	requestURL := c.address + common.DownloadURL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)

	if err != nil {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("create: %w", err))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("do: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("status: %d", resp.StatusCode))
	}

	count, err := common.Read(ctx, resp.Body, common.DefaultBufSize)
	if err != nil {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("read: %w", err))
	}

	slog.Debug("reads", "count", common.ByteSize(count))
	return int64(count), resp.Body.Close()
}

// upload sends data to server.
func (c *Client) upload(ctx context.Context, client *http.Client) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body := common.NewReader(ctx, common.DefaultBufSize)
	requestURL := c.address + common.UploadURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, body)

	if err != nil {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("create: %w", err))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("do: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		return 0, errors.Join(ErrRequestFailed, fmt.Errorf("status: %d", resp.StatusCode))
	}

	count := body.Count.Load()
	slog.Debug("writes", "count", common.ByteSize(int(count)))

	return count, resp.Body.Close()
}

// progress prints a dot every d duration until the returned channel is closed.
func progress(d time.Duration) chan<- struct{} {
	var (
		ticker = time.NewTicker(d)
		stop   = make(chan struct{})
	)

	go func() {
		for {
			select {
			case <-ticker.C:
				print(". ")
			case <-stop:
				ticker.Stop()
				return
			}
		}
	}()

	return stop
}
