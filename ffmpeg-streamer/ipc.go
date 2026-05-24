package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
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

// ipcReader connects to the tcp-camera-backend Unix socket and maintains the
// latest JPEG frame for each active camera. It reconnects automatically and
// clears all frames when the IPC connection is lost.
type ipcReader struct {
	socketPath string
	logger     *log.Logger

	mu      sync.RWMutex
	frames  map[uint32][]byte
	version atomic.Uint64
}

func newIPCReader(socketPath string, logger *log.Logger) *ipcReader {
	return &ipcReader{
		socketPath: socketPath,
		logger:     logger,
		frames:     make(map[uint32][]byte),
	}
}

func (r *ipcReader) run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := r.connect(ctx); err != nil && ctx.Err() == nil {
			r.logger.Printf("IPC disconnected (%v), retrying in %s", err, ipcReconnDelay)
		}

		// Lost the connection — wipe frames so ffmpeg doesn't loop stale data.
		r.mu.Lock()
		r.frames = make(map[uint32][]byte)
		r.mu.Unlock()
		r.version.Add(1)

		select {
		case <-ctx.Done():
			return
		case <-time.After(ipcReconnDelay):
		}
	}
}

func (r *ipcReader) connect(ctx context.Context) error {
	conn, err := net.Dial("unix", r.socketPath)
	if err != nil {
		return fmt.Errorf("dial %s: %w", r.socketPath, err)
	}
	defer conn.Close()

	r.logger.Printf("IPC connected to %s", r.socketPath)

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
			r.mu.Lock()
			r.frames[cameraID] = payload
			r.mu.Unlock()
			r.version.Add(1)
		case ipcMsgOffline:
			r.mu.Lock()
			delete(r.frames, cameraID)
			r.mu.Unlock()
			r.version.Add(1)
		}
	}
}

// latest returns the most recent JPEG for a single camera, or nil if none.
func (r *ipcReader) latest(cameraID uint32) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frames[cameraID]
}

// allLatest returns a snapshot of the latest JPEG for every active camera.
func (r *ipcReader) allLatest() map[uint32][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make(map[uint32][]byte, len(r.frames))
	for id, f := range r.frames {
		cp[id] = f
	}
	return cp
}

// currentVersion returns a monotonically increasing counter that increments
// whenever any frame changes. Use it to avoid redundant canvas recompositions.
func (r *ipcReader) currentVersion() uint64 {
	return r.version.Load()
}
