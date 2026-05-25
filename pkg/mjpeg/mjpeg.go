// Package mjpeg provides helpers for writing MJPEG (multipart/x-mixed-replace) HTTP streams.
package mjpeg

import (
	"fmt"
	"io"
)

const (
	Boundary    = "instachron"
	ContentType = "multipart/x-mixed-replace;boundary=" + Boundary
)

// WriteFrame writes one MJPEG multipart frame to w.
// The caller is responsible for flushing w after the call.
func WriteFrame(w io.Writer, jpeg []byte) error {
	if _, err := fmt.Fprintf(w, "--%s\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n",
		Boundary, len(jpeg)); err != nil {
		return err
	}
	if _, err := w.Write(jpeg); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "\r\n")
	return err
}
