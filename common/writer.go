package common

import (
	"context"
	"errors"
)

// ErrWriterTimeout is returned when the writer timeout.
var ErrWriterTimeout = errors.New("writer timeout")

// Writer is a writer that writes nothing.
type Writer struct {
	errChan chan error
}

func NewWriter(ctx context.Context) *Writer {
	w := &Writer{errChan: make(chan error)}

	go func() {
		defer close(w.errChan)

		for {
			select {
			case <-ctx.Done():
				if err := ctx.Err(); errors.Is(err, context.DeadlineExceeded) {
					w.errChan <- ErrWriterTimeout
				} else {
					w.errChan <- err
				}
				return
			default:
				w.errChan <- nil
			}
		}
	}()

	return w
}

func (w *Writer) Write(p []byte) (int, error) {
	if err := <-w.errChan; err != nil {
		return 0, err
	}

	return len(p), nil
}
