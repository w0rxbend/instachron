// Package frameipc owns the binary wire format shared between tcp-camera-backend
// (writer) and every IPC consumer (camera-web-api, ffmpeg-streamer).
//
// Wire format: magic(2) | type(1) | cameraID(4 BE) | payloadSize(4 BE) | payload
package frameipc

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	Magic1 byte = 0xAA
	Magic2 byte = 0xBB

	TypeFrame   byte = 0x01
	TypeOffline byte = 0x02

	HeaderSize = 11 // 2 magic + 1 type + 4 cameraID + 4 payloadSize
)

// Msg is a single IPC message.
type Msg struct {
	Type     byte
	CameraID uint32
	Payload  []byte // nil for TypeOffline
}

// Write serialises m to w. It performs two Write calls: one for the fixed
// header and one for the payload (skipped when empty).
func Write(w io.Writer, m Msg) error {
	var hdr [HeaderSize]byte
	hdr[0] = Magic1
	hdr[1] = Magic2
	hdr[2] = m.Type
	binary.BigEndian.PutUint32(hdr[3:7], m.CameraID)
	binary.BigEndian.PutUint32(hdr[7:11], uint32(len(m.Payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if len(m.Payload) > 0 {
		if _, err := w.Write(m.Payload); err != nil {
			return err
		}
	}
	return nil
}

// Read reads exactly one message from r. It blocks until the full message
// (header + payload) is available or an error occurs.
func Read(r io.Reader) (Msg, error) {
	var hdr [HeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Msg{}, err
	}
	if hdr[0] != Magic1 || hdr[1] != Magic2 {
		return Msg{}, fmt.Errorf("frameipc: bad magic 0x%02x%02x", hdr[0], hdr[1])
	}

	msgType := hdr[2]
	cameraID := binary.BigEndian.Uint32(hdr[3:7])
	payloadSize := binary.BigEndian.Uint32(hdr[7:11])

	var payload []byte
	if payloadSize > 0 {
		payload = make([]byte, payloadSize)
		if _, err := io.ReadFull(r, payload); err != nil {
			return Msg{}, fmt.Errorf("frameipc: read payload camera=%d: %w", cameraID, err)
		}
	}

	return Msg{Type: msgType, CameraID: cameraID, Payload: payload}, nil
}
