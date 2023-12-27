package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Data size constants.
const (
	KB float64 = 1024
	MB         = 1024 * KB
	GB         = 1024 * MB
)

// SpeedUnit is a speed unit type.
type SpeedUnit uint8

// Available speed units.
const (
	SpeedMicroseconds SpeedUnit = iota
	SpeedMilliseconds
	SpeedSeconds
)

const (
	// MaxPortNumber is a maximum port number.
	MaxPortNumber uint64 = 65535
)

var (
	// ErrInvalidPort is returned when the port number is invalid.
	ErrInvalidPort = errors.New("invalid port number")

	// ErrIPAddress is returned when the remote address is not available.
	ErrIPAddress = errors.New("failed to get remote address")

	// ErrStop is returned when the program must be stopped. It's not an error, only a signal.
	ErrStop = errors.New("stop")
)

// Starter is a program start interface.
type Starter interface {
	Start(ctx context.Context) error
}

// ParsePort parses a port number.
func ParsePort(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("parse port: %w", err)
	}

	if port < 1 || port > MaxPortNumber {
		return 0, fmt.Errorf("port number must be in range [1, %d]", MaxPortNumber)
	}

	return uint16(port), nil
}

// Params is a program parameters.
type Params struct {
	Host    string
	Port    uint16
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

// Address returns a network address.
func (p *Params) Address() string {
	return net.JoinHostPort(p.Host, strconv.FormatUint(uint64(p.Port), 10))
}

// ByteSize returns generate size as a string.
func ByteSize(size uint64) string {
	var floatSize = float64(size)

	switch {
	case floatSize < KB:
		return fmt.Sprintf("%.0f B", floatSize)
	case floatSize < MB:
		return fmt.Sprintf("%.2f KB", floatSize/KB)
	case floatSize < GB:
		return fmt.Sprintf("%.2f MB", floatSize/MB)
	default:
		return fmt.Sprintf("%.2f GB", floatSize/GB)
	}
}

// Speed returns network speed as a string.
func Speed(duration time.Duration, count uint64, unit SpeedUnit) string {
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
		bitsCount := count * 8
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

// SkipError skips some errors or returns original one.
func SkipError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}

	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return err
	}

	errMsg := opErr.Error()
	ignoredErrors := []string{"connection reset by peer", "broken pipe", "i/o timeout"}

	for _, e := range ignoredErrors {
		if strings.Contains(errMsg, e) {
			return nil
		}
	}

	return err
}
