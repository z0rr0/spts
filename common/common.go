package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
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
	// MaxPortNumber is a maximum port number.
	MaxPortNumber uint64 = 65535

	// DefaultBufSize is a default buffer size for generate transfer.
	DefaultBufSize = 32 * KB

	// XRequestIPHeader is a header name for client's IP address.
	XRequestIPHeader = "X-Request-IP"
)

var (
	// ErrInvalidPort is returned when the port number is invalid.
	ErrInvalidPort = errors.New("invalid port number")
)

// Starter is a program start interface.
type Starter interface {
	Start(ctx context.Context) error
}

// Params is a program parameters.
type Params struct {
	Host    string
	Port    uint64
	Timeout time.Duration
	Clients int
	Dot     bool
}

// NewLine returns a new line string by dot flag.
func (p *Params) NewLine() string {
	if p.Dot {
		return "\n"
	}
	return ""
}

// ServiceBase is a base client/server struct.
type ServiceBase struct {
	Address string
	*Params
}

// Address returns a valid address.
func Address(host string, port uint64) (string, error) {
	if port < 1 || port > MaxPortNumber {
		return "", errors.Join(ErrInvalidPort, fmt.Errorf("port must be between 1 and %d", MaxPortNumber))
	}

	return net.JoinHostPort(host, strconv.FormatUint(port, 10)), nil
}

// URL returns a valid client's URL.
func URL(host string, port uint64) (string, error) {
	scheme := "http"

	if port == 443 {
		scheme = "https"
	} else {
		address, err := Address(host, port)
		if err != nil {
			return "", err
		}

		host = address
	}

	u := url.URL{Host: host, Scheme: scheme}
	return strings.TrimRight(u.String(), "/ "), nil
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
func Speed(duration time.Duration, biyteScount int64, unit SpeedUnit) string {
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
		bitsCount := biyteScount * 8
		speed = float64(bitsCount) / speed
	}

	switch {
	case speed < KB:
		return fmt.Sprintf("%.2f Bits/%s", speed, name)
	case speed < MB:
		return fmt.Sprintf("%.2f KBits/%s", speed/KB, name)
	case speed < GB:
		return fmt.Sprintf("%.2f MBits/%s", speed/MB, name)
	default:
		return fmt.Sprintf("%.2f GBits/%s", speed/GB, name)
	}
}
