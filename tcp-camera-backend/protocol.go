package main

import (
	"encoding/binary"
	"fmt"
)

const (
	frameMagic      uint32 = 0x4A504753
	frameHeaderSize        = 16
)

type frameHeader struct {
	Sequence    uint32
	PayloadSize uint32
	TimestampMs uint32
}

func parseFrameHeader(header []byte) (frameHeader, error) {
	if len(header) != frameHeaderSize {
		return frameHeader{}, fmt.Errorf("invalid header size: got %d, want %d", len(header), frameHeaderSize)
	}

	magic := binary.BigEndian.Uint32(header[0:4])
	if magic != frameMagic {
		return frameHeader{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}

	return frameHeader{
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
