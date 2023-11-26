package common

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRead(t *testing.T) {
	tests := []struct {
		name      string
		reader    io.Reader
		bufSize   int
		ctxFunc   func() context.Context
		expected  int
		withError bool
	}{
		{
			name:     "normal",
			reader:   strings.NewReader("test"),
			bufSize:  2,
			ctxFunc:  func() context.Context { return context.Background() },
			expected: 4,
		},
		{
			name:    "timeout",
			reader:  strings.NewReader("test"),
			bufSize: 2,
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
				defer cancel()
				time.Sleep(5 * time.Millisecond)
				return ctx
			},
		},
		{
			name:    "cancel",
			reader:  strings.NewReader("test"),
			bufSize: 2,
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			withError: true,
		},
		{
			name:    "eof",
			reader:  strings.NewReader(""),
			bufSize: 2,
			ctxFunc: func() context.Context { return context.Background() },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctxFunc()
			bytesRead, err := Read(ctx, tc.reader, tc.bufSize)

			if (err != nil) != tc.withError {
				t.Errorf("expected error: %v, got: %v", tc.withError, err)
				return
			}

			if bytesRead != tc.expected {
				t.Errorf("expected %d bytes read, got %d", tc.expected, bytesRead)
			}
		})
	}
}

func TestNewReader(t *testing.T) {
	tests := []struct {
		name      string
		bufSize   int
		ctxFunc   func() context.Context
		expected  int
		withError bool
	}{
		{
			name:      "normal",
			bufSize:   10,
			ctxFunc:   func() context.Context { return context.Background() },
			withError: false,
			expected:  10,
		},
		{
			name:    "canceled",
			bufSize: 10,
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			withError: true,
		},
		{
			name:    "timeout",
			bufSize: 10,
			ctxFunc: func() context.Context {
				ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
				defer cancel()
				time.Sleep(5 * time.Millisecond)
				return ctx
			},
			withError: true,
		},
		{
			name:     "default",
			bufSize:  DefaultBufSize,
			ctxFunc:  func() context.Context { return context.Background() },
			expected: DefaultBufSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctxFunc()
			r := NewReader(ctx)

			p := make([]byte, tc.bufSize)
			n, err := r.Read(p)

			if (err != nil) != tc.withError {
				t.Errorf("expected error: %v, got: %v", tc.withError, err)
				return
			}

			if n != tc.expected {
				t.Errorf("expected %d bytes read, got %d", tc.expected, n)
			}
		})
	}
}
