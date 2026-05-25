package protocol

import (
	"encoding/binary"
	"testing"
)

func TestParseFrameHeader(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicLegacy)
	binary.BigEndian.PutUint32(header[4:8], 42)
	binary.BigEndian.PutUint32(header[8:12], 123456)
	binary.BigEndian.PutUint32(header[12:16], 987654321)

	got, err := ParseLegacyHeader(header)
	if err != nil {
		t.Fatalf("ParseLegacyHeader returned error: %v", err)
	}

	if got.Sequence != 42 {
		t.Fatalf("Sequence = %d, want 42", got.Sequence)
	}
	if got.CameraID != DefaultCameraID {
		t.Fatalf("CameraID = %d, want %d", got.CameraID, DefaultCameraID)
	}
	if got.PayloadSize != 123456 {
		t.Fatalf("PayloadSize = %d, want 123456", got.PayloadSize)
	}
	if got.TimestampMs != 987654321 {
		t.Fatalf("TimestampMs = %d, want 987654321", got.TimestampMs)
	}
}

func TestParseFrameHeaderWithCameraID(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicWithDevice)
	binary.BigEndian.PutUint32(header[4:8], 42)
	binary.BigEndian.PutUint32(header[8:12], 123456)
	binary.BigEndian.PutUint32(header[12:16], 987654321)
	cameraID := make([]byte, CameraIDSize)
	binary.BigEndian.PutUint32(cameraID, 17)

	got, err := ParseDeviceHeader(header, cameraID)
	if err != nil {
		t.Fatalf("ParseDeviceHeader returned error: %v", err)
	}

	if got.CameraID != 17 {
		t.Fatalf("CameraID = %d, want 17", got.CameraID)
	}
	if got.Sequence != 42 {
		t.Fatalf("Sequence = %d, want 42", got.Sequence)
	}
	if got.PayloadSize != 123456 {
		t.Fatalf("PayloadSize = %d, want 123456", got.PayloadSize)
	}
	if got.TimestampMs != 987654321 {
		t.Fatalf("TimestampMs = %d, want 987654321", got.TimestampMs)
	}
}

func TestParseFrameHeaderRejectsBadMagic(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], 0xDEADBEEF)

	if _, err := ParseLegacyHeader(header); err == nil {
		t.Fatal("ParseLegacyHeader returned nil error for bad magic")
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	validJPEG := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}
	if !LooksLikeJPEG(validJPEG) {
		t.Fatal("LooksLikeJPEG returned false for valid JPEG markers")
	}

	invalidJPEG := []byte{0x00, 0xD8, 0xAA, 0xBB, 0xFF, 0x00}
	if LooksLikeJPEG(invalidJPEG) {
		t.Fatal("LooksLikeJPEG returned true for invalid JPEG markers")
	}
}

func TestParseFrameHeaderRejectsJPGD(t *testing.T) {
	header := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(header[0:4], MagicWithDevice)

	if _, err := ParseLegacyHeader(header); err == nil {
		t.Fatal("ParseLegacyHeader should reject JPGD magic")
	}
}
