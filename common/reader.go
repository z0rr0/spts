package common

import (
	"context"
	"errors"
	"io"
	"math/rand"
	"time"
)

// Reader is a reader that reads random generate.
type Reader struct {
	rnd     *rand.Rand
	errChan chan error
}

// NewReader returns a new Reader that reads random generate
// with the given buffer size until the context is canceled or timed out.
func NewReader(ctx context.Context) *Reader {
	r := &Reader{
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())), //#nosec G404 - this data is not security sensitive
		errChan: make(chan error),
	}

	go func() {
		defer close(r.errChan)

		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
					r.errChan <- io.EOF
				} else {
					r.errChan <- err
				}
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
	if err := <-r.errChan; err != nil {
		return 0, err
	}

	return r.rnd.Read(p)
}
