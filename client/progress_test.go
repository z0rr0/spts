package client

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"
)

func TestProgress(t *testing.T) {
	var b bytes.Buffer

	prg := newProgress(&b, 25*time.Millisecond)

	time.Sleep(120 * time.Millisecond)
	prg.done()

	s := b.String()
	expected := ". . . . "
	if s != expected {
		t.Errorf("want %s, got %s", expected, s)
	}
}

func TestProgressWriter(t *testing.T) {
	var expectedWriter = &bytes.Buffer{}

	ctx := context.WithValue(context.Background(), ctxWriterKey, expectedWriter)
	writer := progressWriter(ctx)

	if _, ok := writer.(*bytes.Buffer); !ok {
		t.Errorf("expected writer to be %T, but got %T", expectedWriter, writer)
	}

	ctx = context.Background()
	writer = progressWriter(ctx)

	if _, ok := writer.(*os.File); !ok {
		t.Errorf("expected writer to be %T, but got %T", os.Stdout, writer)
	}
}
