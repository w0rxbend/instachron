package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

const (
	ipcMagic1      byte = 0xAA
	ipcMagic2      byte = 0xBB
	ipcMsgFrame    byte = 0x01
	ipcMsgOffline  byte = 0x02
	ipcHeaderSize       = 11 // 2 magic + 1 type + 4 cameraID + 4 payloadSize
	ipcReconnDelay      = time.Second
)

// socketReader connects to the tcp-camera-backend IPC Unix socket and dispatches
// incoming messages to the hub manager. It reconnects automatically on failure
// and marks all cameras offline whenever the IPC connection is lost.
type socketReader struct {
	socketPath string
	manager    *hubManager
	logger     *log.Logger
}

func newSocketReader(socketPath string, manager *hubManager, logger *log.Logger) *socketReader {
	return &socketReader{
		socketPath: socketPath,
		manager:    manager,
		logger:     logger,
	}
}

func (r *socketReader) run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := r.connect(ctx); err != nil && ctx.Err() == nil {
			r.logger.Printf("IPC disconnected (%v), retrying in %s", err, ipcReconnDelay)
		}

		// All cameras are now unreachable — mark them offline so the API
		// reflects reality while we wait to reconnect.
		r.manager.markAllOffline()

		select {
		case <-ctx.Done():
			return
		case <-time.After(ipcReconnDelay):
		}
	}
}

func (r *socketReader) connect(ctx context.Context) error {
	conn, err := net.Dial("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("dial %s: %w", r.socketPath, err)
	}
	defer conn.Close()

	r.logger.Printf("IPC connected to %s", r.socketPath)

	// Close the connection when the context is cancelled so the read loop unblocks.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	hdr := make([]byte, ipcHeaderSize)
	for {
		if _, err := io.ReadFull(conn, hdr); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read header: %w", err)
		}

		if hdr[0] != ipcMagic1 || hdr[1] != ipcMagic2 {
			return fmt.Errorf("stream desynced: bad magic 0x%02x%02x", hdr[0], hdr[1])
		}

		msgType := hdr[2]
		cameraID := binary.BigEndian.Uint32(hdr[3:7])
		payloadSize := binary.BigEndian.Uint32(hdr[7:11])

		var payload []byte
		if payloadSize > 0 {
			payload = make([]byte, payloadSize)
			if _, err := io.ReadFull(conn, payload); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("read payload camera=%d: %w", cameraID, err)
			}
		}

		switch msgType {
		case ipcMsgFrame:
			r.manager.dispatch(cameraID, payload)
		case ipcMsgOffline:
			r.manager.markOffline(cameraID)
		default:
			r.logger.Printf("unknown IPC message type 0x%02x for camera=%d, skipping", msgType, cameraID)
		}
	}
}
