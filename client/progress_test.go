package client

import (
	"bytes"
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
