package main

import (
	"encoding/binary"
	"testing"
)

func TestParseFrameHeader(t *testing.T) {
	header := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(header[0:4], frameMagic)
	binary.BigEndian.PutUint32(header[4:8], 42)
	binary.BigEndian.PutUint32(header[8:12], 123456)
	binary.BigEndian.PutUint32(header[12:16], 987654321)

	got, err := parseFrameHeader(header)
	if err != nil {
		t.Fatalf("parseFrameHeader returned error: %v", err)
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
	header := make([]byte, frameHeaderSize)
	binary.BigEndian.PutUint32(header[0:4], 0xDEADBEEF)

	if _, err := parseFrameHeader(header); err == nil {
		t.Fatal("parseFrameHeader returned nil error for bad magic")
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	validJPEG := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}
	if !looksLikeJPEG(validJPEG) {
		t.Fatal("looksLikeJPEG returned false for valid JPEG markers")
	}

	invalidJPEG := []byte{0x00, 0xD8, 0xAA, 0xBB, 0xFF, 0x00}
	if looksLikeJPEG(invalidJPEG) {
		t.Fatal("looksLikeJPEG returned true for invalid JPEG markers")
	}
}
