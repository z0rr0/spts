package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Data size constants.
const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

// Server's URLs
const (
	UploadURL   = "/upload"
	DownloadURL = "/download"
)

const (
	maxPortNumber  uint64 = 65535
	DefaultBufSize        = 65 * KB
)

var (
	// ErrInvalidPort is returned when the port number is invalid.
	ErrInvalidPort = errors.New("invalid port number")
)

// Mode is a program mode.
type Mode interface {
	Start(ctx context.Context) error
}

// Address returns a valid address.
func Address(host string, port uint64) (string, error) {
	if port < 1 || port > maxPortNumber {
		return "", errors.Join(ErrInvalidPort, fmt.Errorf("port must be between 1 and %d", maxPortNumber))
	}

	return net.JoinHostPort(host, strconv.FormatUint(port, 10)), nil
}

// ByteSize returns data size as a string.
func ByteSize(size int) string {
	switch {
	case size < KB:
		return fmt.Sprintf("%d B", size)
	case size < MB:
		return fmt.Sprintf("%.2f KB", float64(size)/KB)
	case size < GB:
		return fmt.Sprintf("%.2f MB", float64(size)/MB)
	default:
		return fmt.Sprintf("%.2f GB", float64(size)/GB)
	}
}

// Speed returns network speed as a string.
func Speed(start time.Time, count int64) string {
	var (
		s        float64
		duration = time.Since(start)
	)

	if seconds := duration.Seconds(); seconds > 0 {
		s = float64(count) / seconds
	}

	switch {
	case s < KB:
		return fmt.Sprintf("%.2f B/s", s)
	case s < MB:
		return fmt.Sprintf("%.2f KB/s", s/KB)
	case s < GB:
		return fmt.Sprintf("%.2f MB/s", s/MB)
	default:
		return fmt.Sprintf("%.2f GB/s", s/GB)
	}
}
