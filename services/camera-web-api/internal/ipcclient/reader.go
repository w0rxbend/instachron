// Package ipcclient connects to the tcp-camera-backend IPC Unix socket and
// dispatches incoming messages to caller-provided handlers.
package ipcclient

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/w0rxbend/instachron/pkg/frameipc"
)

const reconnDelay = time.Second

// Handler receives decoded IPC messages.
type Handler struct {
	// OnFrame is called for each received JPEG frame.
	OnFrame func(cameraID uint32, jpeg []byte)
	// OnOffline is called when the backend signals a camera went offline.
	OnOffline func(cameraID uint32)
	// OnDisconnect is called whenever the IPC connection is lost, before the
	// reconnect delay. Use it to mark all cameras offline.
	OnDisconnect func()
}

// Reader connects to the IPC socket and dispatches messages until ctx is done.
// It reconnects automatically with a fixed delay on every disconnect.
type Reader struct {
	socketPath string
	handler    Handler
	logger     *log.Logger
}

// New returns a Reader that dispatches to handler.
func New(socketPath string, handler Handler, logger *log.Logger) *Reader {
	return &Reader{socketPath: socketPath, handler: handler, logger: logger}
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

		if r.handler.OnDisconnect != nil {
			r.handler.OnDisconnect()
		}

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
			if r.handler.OnFrame != nil {
				r.handler.OnFrame(msg.CameraID, msg.Payload)
			}
		case frameipc.TypeOffline:
			if r.handler.OnOffline != nil {
				r.handler.OnOffline(msg.CameraID)
			}
		default:
			r.logger.Printf("IPC: unknown message type 0x%02x for camera=%d, skipping", msg.Type, msg.CameraID)
		}
	}
}
