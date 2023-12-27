package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestAddress(t *testing.T) {
	testCases := []struct {
		host string
		port uint16
		want string
	}{
		{host: "localhost", port: 8080, want: "localhost:8080"},
		{host: "127.0.0.1", port: 8081, want: "127.0.0.1:8081"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			params := Params{Host: tc.host, Port: tc.port}
			if got := params.Address(); got != tc.want {
				t.Errorf("want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestNewLine(t *testing.T) {
	params := Params{Dot: true}

	if nl := params.NewLine(); nl != "\n" {
		t.Errorf("want newline, got %q", nl)
	}

	params.Dot = false

	if nl := params.NewLine(); nl != "" {
		t.Errorf("want empty string, got %q", nl)
	}
}

func TestByteSize(t *testing.T) {
	testCases := []struct {
		size uint64
		want string
	}{
		{size: 0, want: "0 B"},
		{size: 1, want: "1 B"},
		{size: 1023, want: "1023 B"},
		{size: uint64(KB), want: "1.00 KB"},
		{size: uint64(KB) + 120, want: "1.12 KB"},
		{size: uint64(MB), want: "1.00 MB"},
		{size: uint64(MB + 130*KB), want: "1.13 MB"},
		{size: uint64(GB), want: "1.00 GB"},
		{size: uint64(10*GB + 140*MB), want: "10.14 GB"},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got := ByteSize(tc.size)

			if got != tc.want {
				t.Errorf("want %s, got %s", tc.want, got)
			}
		})
	}
}

func TestSpeed(t *testing.T) {
	var delay = 10 * time.Millisecond

	testCases := []struct {
		name     string
		duration time.Duration
		count    uint64
		unit     SpeedUnit
		want     string
	}{
		{name: "zero_with_seconds", duration: 0, count: 0, unit: SpeedSeconds, want: "0.00 Bits/s"},
		{name: "zero_with_milliseconds", duration: 0, count: 0, unit: SpeedMilliseconds, want: "0.00 Bits/ms"},
		{name: "zero_with_microseconds", duration: 0, count: 0, unit: SpeedMicroseconds, want: "0.00 Bits/μs"},
		{name: "microseconds", duration: 5000 * time.Microsecond, count: 50, unit: SpeedMicroseconds, want: "0.08 Bits/μs"},
		{name: "milliseconds", duration: delay, count: 57, unit: SpeedMilliseconds, want: "45.60 Bits/ms"},
		{name: "seconds", duration: time.Second, count: 21, unit: SpeedSeconds, want: "168.00 Bits/s"},
		{name: "kilobytes", duration: delay, count: 105 * uint64(KB), unit: SpeedMilliseconds, want: "84.00 KBits/ms"},
		{name: "megabytes", duration: delay, count: 106 * uint64(MB), unit: SpeedMilliseconds, want: "84.80 MBits/ms"},
		{name: "gigabytes", duration: delay, count: 107 * uint64(GB), unit: SpeedMilliseconds, want: "85.60 GBits/ms"},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			if got := Speed(tc.duration, tc.count, tc.unit); got != tc.want {
				t.Errorf("withError %s, got %s", tc.want, got)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		want      uint16
		withError bool
	}{
		{name: "zero", value: "0", withError: true},
		{name: "negative", value: "-1", withError: true},
		{name: "too_big", value: "65536", withError: true},
		{name: "success", value: "8080", want: 8080},
		{name: "not_number", value: "abc", withError: true},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePort(tc.value)
			if err != nil {
				if !tc.withError {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if got != tc.want {
				t.Errorf("expected %d, got %d", tc.want, got)
			}
		})
	}
}

func TestSkipError(t *testing.T) {
	var (
		someError       = errors.New("some error")
		opError         = &net.OpError{Err: someError}
		opErrorSkipPeer = &net.OpError{Err: errors.New("oops, connection reset by peer")}
		opErrorSkipPipe = &net.OpError{Err: errors.New("oops, broken pipe again")}
	)

	testCases := []struct {
		name     string
		err      error
		expected error
	}{
		{name: "nil"},
		{name: "eof", err: io.EOF},
		{name: "other", err: someError, expected: someError},
		{name: "inherit", err: fmt.Errorf("some error: %w", io.EOF)},
		{name: "skip_net_error_peer", err: opErrorSkipPeer},
		{name: "skip_net_error_pipe", err: opErrorSkipPipe},
		{name: "op_error", err: opError, expected: someError},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			err := SkipError(tc.err)

			if err != nil && !errors.Is(err, tc.expected) {
				t.Errorf("expected %v, got %v", tc.expected, err)
				return
			}

			if err == nil && tc.expected != nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestNewReader(t *testing.T) {
	tests := []struct {
		name      string
		ctxFunc   func() context.Context
		withError bool
	}{
		{
			name:      "normal",
			ctxFunc:   func() context.Context { return context.Background() },
			withError: false,
		},
		{
			name: "canceled",
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			withError: true,
		},
		{
			name: "timeout",
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
				defer func() {
					time.Sleep(15 * time.Millisecond)
					cancel()
				}()
				time.Sleep(10 * time.Millisecond)
				return ctx
			},
			withError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const bufSize = 10

			ctx := tc.ctxFunc()
			r := NewReader(ctx)

			p := make([]byte, bufSize)
			n, err := r.Read(p)

			if err != nil {
				if !tc.withError {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if n != bufSize {
				t.Errorf("expected %d bytes read, got %d", bufSize, n)
			}
		})
	}
}

func TestNewWriter(t *testing.T) {
	testCases := []struct {
		name      string
		ctxFunc   func() context.Context
		withError bool
	}{
		{
			name:      "normal",
			ctxFunc:   func() context.Context { return context.Background() },
			withError: false,
		},
		{
			name: "canceled",
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			withError: true,
		},
		{
			name: "timeout",
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
				defer func() {
					time.Sleep(15 * time.Millisecond)
					cancel()
				}()
				time.Sleep(10 * time.Millisecond)
				return ctx
			},
			withError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			const bufSize = 10

			ctx := tc.ctxFunc()
			w := NewWriter(ctx)

			p := make([]byte, bufSize)
			n, err := w.Write(p)

			if err != nil {
				if !tc.withError {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}

			if n != bufSize {
				t.Errorf("expected %d bytes written, got %d", bufSize, n)
			}
		})
	}
}
