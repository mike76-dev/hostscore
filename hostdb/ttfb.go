package hostdb

import (
	"io"
	"time"
)

// A ttfbWriter is an io.Writer that records the time between initialization
// and when the first byte is written to it.
type ttfbWriter struct {
	w     io.Writer
	start time.Time
	write time.Time
}

func (w *ttfbWriter) Write(p []byte) (int, error) {
	if w.write == (time.Time{}) {
		w.write = time.Now()
	}
	return w.w.Write(p)
}

func (w *ttfbWriter) TTFB() time.Duration {
	return w.write.Sub(w.start)
}

func newTTFBWriter(w io.Writer) *ttfbWriter {
	return &ttfbWriter{w: w, start: time.Now()}
}
