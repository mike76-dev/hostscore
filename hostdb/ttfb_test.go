package hostdb

import (
	"bytes"
	"testing"
	"time"
)

func TestTTFBWriter(t *testing.T) {
	var buf bytes.Buffer
	w := newTTFBWriter(&buf)

	time.Sleep(10 * time.Millisecond)
	if n, err := w.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write = (%d, %v), want (5, nil)", n, err)
	}
	ttfb := w.TTFB()
	if ttfb < 10*time.Millisecond {
		t.Errorf("TTFB = %v, want at least 10ms", ttfb)
	}

	// Later writes must not change the recorded time of the first byte.
	time.Sleep(10 * time.Millisecond)
	if _, err := w.Write([]byte(" world")); err != nil {
		t.Fatal(err)
	}
	if got := w.TTFB(); got != ttfb {
		t.Errorf("TTFB changed after a second write: %v != %v", got, ttfb)
	}

	if buf.String() != "hello world" {
		t.Errorf("underlying writer got %q, want %q", buf.String(), "hello world")
	}
}
