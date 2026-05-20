package main

import (
	"encoding/binary"
	"fmt"
)

const (
	frameMagicLegacy     uint32 = 0x4A504753 // JPGS
	frameMagicWithDevice uint32 = 0x4A504744 // JPGD

	frameHeaderSize   = 16
	frameCameraIDSize = 4
	defaultCameraID   = 0
)

type frameHeader struct {
	CameraID    uint32
	Sequence    uint32
	PayloadSize uint32
	TimestampMs uint32
}

// parseFrameHeader parses a legacy JPGS frame header.
// It only accepts frameMagicLegacy; callers must route JPGD frames to
// parseFrameHeaderWithCameraID to avoid leaving camera-id bytes unread on the stream.
func parseFrameHeader(header []byte) (frameHeader, error) {
	if len(header) != frameHeaderSize {
		return frameHeader{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), frameHeaderSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != frameMagicLegacy {
		return frameHeader{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return frameHeader{
		CameraID:    defaultCameraID,
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

// parseFrameHeaderWithCameraID parses a JPGD frame header plus its camera-id bytes.
func parseFrameHeaderWithCameraID(header []byte, cameraIDBytes []byte) (frameHeader, error) {
	if len(header) != frameHeaderSize {
		return frameHeader{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), frameHeaderSize)
	}
	if len(cameraIDBytes) != frameCameraIDSize {
		return frameHeader{}, fmt.Errorf("invalid camera id size: got %d, want %d", len(cameraIDBytes), frameCameraIDSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != frameMagicWithDevice {
		return frameHeader{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return frameHeader{
		CameraID:    binary.BigEndian.Uint32(cameraIDBytes),
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

func looksLikeJPEG(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}

	return payload[0] == 0xFF &&
		payload[1] == 0xD8 &&
		payload[len(payload)-2] == 0xFF &&
		payload[len(payload)-1] == 0xD9
}
