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

// SpeedUnit is a speed unit type.
type SpeedUnit uint8

// Available speed units.
const (
	SpeedMicroseconds SpeedUnit = iota
	SpeedMilliseconds
	SpeedSeconds
)

// Server's URLs
const (
	UploadURL   = "/upload"
	DownloadURL = "/download"
)

const (
	maxPortNumber uint64 = 65535

	// DefaultBufSize is a default buffer size for generate transfer.
	DefaultBufSize = 32 * KB
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

// ByteSize returns generate size as a string.
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

// Speed returns network speed as a string and
func Speed(duration time.Duration, count int64, unit SpeedUnit) string {
	var (
		speed float64
		name  = "s"
	)

	switch unit {
	case SpeedMicroseconds:
		speed = float64(duration.Microseconds())
		name = "μs"
	case SpeedMilliseconds:
		speed = float64(duration.Milliseconds())
		name = "ms"
	default:
		speed = duration.Seconds()
	}

	if speed > 0 {
		speed = float64(count) / speed
	}

	switch {
	case speed < KB:
		return fmt.Sprintf("%.2f B/%s", speed, name)
	case speed < MB:
		return fmt.Sprintf("%.2f KB/%s", speed/KB, name)
	case speed < GB:
		return fmt.Sprintf("%.2f MB/%s", speed/MB, name)
	default:
		return fmt.Sprintf("%.2f GB/%s", speed/GB, name)
	}
}
