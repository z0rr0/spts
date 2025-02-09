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
	// TimeoutMultiplier is a multiplier for timeout to read all data.
	TimeoutMultiplier time.Duration = 5
)

var (
	// ErrIPAddress is returned when the remote address is not available.
	ErrIPAddress = errors.New("failed to get remote address")
)

// Params is a program parameters.
type Params struct {
	Host    string
	Port    uint16
	Timeout time.Duration
	Clients int
	Dot     bool
	Chunk   uint32
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
	var ignoredErrors = [3]string{"connection reset by peer", "broken pipe", "i/o timeout"}

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
	for _, ignoredErr := range ignoredErrors {
		if strings.Contains(errMsg, ignoredErr) {
			return nil
		}
	}

	return err
}
