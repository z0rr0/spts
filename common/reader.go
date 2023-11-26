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

type payload struct {
	p   []byte
	err error
}

// Reader is a reader that reads random generate.
type Reader struct {
	bufSize int
	rnd     *rand.Rand
	dataCh  chan payload
	Count   atomic.Int64 // total read bytes
}

// NewReader returns a new Reader that reads random generate
// with the given buffer size until the context is canceled or timed out.
func NewReader(ctx context.Context, bufSize int) *Reader {
	r := &Reader{
		bufSize: bufSize,
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
		dataCh:  make(chan payload),
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
					r.dataCh <- payload{err: io.EOF}
				} else {
					r.dataCh <- payload{err: err}
				}

				close(r.dataCh)
				return
			default:
				r.dataCh <- r.generate()
			}
		}
	}()

	return r
}

func (r *Reader) generate() payload {
	p := make([]byte, r.bufSize)
	n, err := r.rnd.Read(p)

	return payload{p: p[:n], err: err}
}

// Read implements the io.Reader interface.
func (r *Reader) Read(p []byte) (int, error) {
	item := <-r.dataCh

	if item.err != nil {
		return 0, item.err
	}

	n := copy(p, item.p)
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
