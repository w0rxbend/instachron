package mjpeg_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/w0rxbend/instachron/shared/mjpeg"
)

func TestWriteFrameContainsBoundary(t *testing.T) {
	var buf bytes.Buffer
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xD9}

	if err := mjpeg.WriteFrame(&buf, jpeg); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "--"+mjpeg.Boundary) {
		t.Errorf("output missing boundary: %q", got)
	}
	if !strings.Contains(got, "Content-Type: image/jpeg") {
		t.Errorf("output missing Content-Type: %q", got)
	}
}

func TestWriteFrameContentLength(t *testing.T) {
	var buf bytes.Buffer
	jpeg := []byte("fakedata")

	if err := mjpeg.WriteFrame(&buf, jpeg); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Content-Length: 8") {
		t.Errorf("output missing correct Content-Length: %q", got)
	}
}

func TestWriteFramePayloadPresent(t *testing.T) {
	var buf bytes.Buffer
	jpeg := []byte{1, 2, 3, 4, 5}

	if err := mjpeg.WriteFrame(&buf, jpeg); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), jpeg) {
		t.Errorf("output missing JPEG payload bytes")
	}
}

func TestContentTypeConstant(t *testing.T) {
	if !strings.Contains(mjpeg.ContentType, mjpeg.Boundary) {
		t.Errorf("ContentType %q does not contain Boundary %q", mjpeg.ContentType, mjpeg.Boundary)
	}
	if !strings.HasPrefix(mjpeg.ContentType, "multipart/x-mixed-replace") {
		t.Errorf("ContentType %q has wrong prefix", mjpeg.ContentType)
	}
}
