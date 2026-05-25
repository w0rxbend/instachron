package protocol

import (
	"encoding/binary"
	"fmt"
)

const (
	MagicLegacy     uint32 = 0x4A504753 // JPGS
	MagicWithDevice uint32 = 0x4A504744 // JPGD

	HeaderSize      = 16
	CameraIDSize    = 4
	DefaultCameraID = 0
)

type Header struct {
	CameraID    uint32
	Sequence    uint32
	PayloadSize uint32
	TimestampMs uint32
}

// ParseLegacyHeader parses a legacy JPGS frame header. It rejects JPGD frames
// so callers do not accidentally leave camera-id bytes unread on the stream.
func ParseLegacyHeader(header []byte) (Header, error) {
	if len(header) != HeaderSize {
		return Header{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), HeaderSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != MagicLegacy {
		return Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return Header{
		CameraID:    DefaultCameraID,
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

// ParseDeviceHeader parses a JPGD frame header plus its camera-id bytes.
func ParseDeviceHeader(header []byte, cameraIDBytes []byte) (Header, error) {
	if len(header) != HeaderSize {
		return Header{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), HeaderSize)
	}
	if len(cameraIDBytes) != CameraIDSize {
		return Header{}, fmt.Errorf("invalid camera id size: got %d, want %d", len(cameraIDBytes), CameraIDSize)
	}
	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != MagicWithDevice {
		return Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
	return Header{
		CameraID:    binary.BigEndian.Uint32(cameraIDBytes),
		Sequence:    binary.BigEndian.Uint32(header[4:8]),
		PayloadSize: binary.BigEndian.Uint32(header[8:12]),
		TimestampMs: binary.BigEndian.Uint32(header[12:16]),
	}, nil
}

func LooksLikeJPEG(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}

	return payload[0] == 0xFF &&
		payload[1] == 0xD8 &&
		payload[len(payload)-2] == 0xFF &&
		payload[len(payload)-1] == 0xD9
}
