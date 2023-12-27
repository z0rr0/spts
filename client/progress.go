package client

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// progress is a progress indicator.
type progress struct {
	stop chan struct{}
	wait chan struct{}
}

// newProgress creates a new progress indicator.
func newProgress(w io.Writer, d time.Duration) *progress {
	var ticker = time.NewTicker(d)

	p := &progress{
		stop: make(chan struct{}),
		wait: make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-ticker.C:
				_, _ = fmt.Fprint(w, ". ")
			case <-p.stop:
				ticker.Stop()
				close(p.wait)
				return
			}
		}
	}()

	return p
}

func (p *progress) done() {
	close(p.stop)
	<-p.wait
}

func progressWriter(ctx context.Context) io.Writer {
	var w io.Writer = os.Stdout

	if ctxWriter, ok := ctx.Value(ctxWriterKey).(io.Writer); ok {
		w = ctxWriter
	}

	return w
}
