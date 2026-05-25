// Package ipcclient connects to the tcp-camera-backend IPC Unix socket and
// maintains the latest JPEG frame for each active camera. Used by ffmpeg-streamer
// to pull the current frame for each camera at a configured frame rate.
package ipcclient

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/w0rxbend/instachron/shared/frameipc"
)

const reconnDelay = time.Second

// Reader connects to the IPC socket and caches the latest JPEG per camera.
// It reconnects automatically and clears all frames when the connection is lost.
type Reader struct {
	socketPath string
	logger     *log.Logger

	mu      sync.RWMutex
	frames  map[uint32][]byte
	version atomic.Uint64
}

// New returns a Reader for the given socket path.
func New(socketPath string, logger *log.Logger) *Reader {
	return &Reader{
		socketPath: socketPath,
		logger:     logger,
		frames:     make(map[uint32][]byte),
	}
}

// Run is the reconnect loop. It blocks until ctx is cancelled.
func (r *Reader) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		if err := r.connect(ctx); err != nil && ctx.Err() == nil {
			r.logger.Printf("IPC disconnected (%v), retrying in %s", err, reconnDelay)
		}

		// Lost the connection — wipe frames so ffmpeg doesn't loop stale data.
		r.mu.Lock()
		r.frames = make(map[uint32][]byte)
		r.mu.Unlock()
		r.version.Add(1)

		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnDelay):
		}
	}
}

func (r *Reader) connect(ctx context.Context) error {
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

	for {
		msg, err := frameipc.Read(conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		switch msg.Type {
		case frameipc.TypeFrame:
			r.mu.Lock()
			r.frames[msg.CameraID] = msg.Payload
			r.mu.Unlock()
			r.version.Add(1)
		case frameipc.TypeOffline:
			r.mu.Lock()
			delete(r.frames, msg.CameraID)
			r.mu.Unlock()
			r.version.Add(1)
		}
	}
}

// Latest returns the most recent JPEG for a single camera, or nil if none.
func (r *Reader) Latest(cameraID uint32) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frames[cameraID]
}

// AllLatest returns a snapshot of the latest JPEG for every active camera.
func (r *Reader) AllLatest() map[uint32][]byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cp := make(map[uint32][]byte, len(r.frames))
	for id, f := range r.frames {
		cp[id] = f
	}
	return cp
}

// CurrentVersion returns a monotonically increasing counter that increments
// whenever any frame changes. Use it to avoid redundant canvas recompositions.
func (r *Reader) CurrentVersion() uint64 {
	return r.version.Load()
}
