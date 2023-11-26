package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sync/atomic"
	"time"
)

// Reader is a reader that reads random generate.
type Reader struct {
	rnd     *rand.Rand
	errChan chan error
	Count   atomic.Int64 // total read bytes
}

// NewReader returns a new Reader that reads random generate
// with the given buffer size until the context is canceled or timed out.
func NewReader(ctx context.Context) *Reader {
	r := &Reader{
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
		errChan: make(chan error),
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
					r.errChan <- io.EOF
				} else {
					r.errChan <- err
				}

				close(r.errChan)
				return
			default:
				r.errChan <- nil
			}
		}
	}()

	return r
}

// Read implements the io.Reader interface.
func (r *Reader) Read(p []byte) (int, error) {
	err := <-r.errChan

	if err != nil {
		return 0, err
	}

	n, err := r.rnd.Read(p)
	if err != nil {
		return 0, err
	}

	r.Count.Add(int64(n))
	return n, nil
}

// Read reads generate from the reader until the context is canceled / timed out or EOF is reached.
// It returns the number of bytes read and an error.
func Read(ctx context.Context, reader io.Reader, bufSize int) (int, error) {
	var (
		total int
		n     int
		err   error
		buf   = make([]byte, bufSize)
	)

	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()

			if errors.Is(err, context.DeadlineExceeded) {
				return total, nil
			}

			return 0, fmt.Errorf("context read error: %w", err)
		default:
			n, err = reader.Read(buf[:])

			if err != nil {
				if errors.Is(err, io.EOF) {
					return total + n, nil
				}
				return 0, fmt.Errorf("read error: %w", err)
			}

			total += n
		}
	}
}
