package restream

import (
	"context"
	"log"
	"net"
	"sync/atomic"
	"time"

	"github.com/w0rxbend/instachron/shared/streamproto"
)

// TCPServerConfig holds parameters for a TCP frame server.
type TCPServerConfig struct {
	ListenAddr   string
	MaxClients   int
	WriteTimeout time.Duration
}

// TCPServer accepts downstream proxy connections and streams frames via
// streamproto. Frames are sourced from a shared Broadcaster; slow clients
// that cannot accept a frame within WriteTimeout are disconnected.
type TCPServer struct {
	cfg         TCPServerConfig
	broadcaster *Broadcaster
	logger      *log.Logger
	clients     atomic.Int64
}

// NewTCPServer creates a TCPServer that publishes from bc.
func NewTCPServer(cfg TCPServerConfig, bc *Broadcaster, logger *log.Logger) *TCPServer {
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 2 * time.Second
	}
	return &TCPServer{cfg: cfg, broadcaster: bc, logger: logger}
}

// Run binds the listener and accepts clients until ctx is cancelled.
func (s *TCPServer) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	s.logger.Printf("TCP stream server listening on %s", s.cfg.ListenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if s.cfg.MaxClients > 0 && int(s.clients.Load()) >= s.cfg.MaxClients {
			s.logger.Printf("TCP: max clients (%d) reached, rejecting %s",
				s.cfg.MaxClients, conn.RemoteAddr())
			conn.Close()
			continue
		}

		s.clients.Add(1)
		go func() {
			defer func() {
				conn.Close()
				s.clients.Add(-1)
			}()
			s.serve(ctx, conn)
		}()
	}
}

func (s *TCPServer) serve(ctx context.Context, conn net.Conn) {
	frames, unsub := s.broadcaster.Subscribe()
	defer unsub()

	w := streamproto.NewWriter(conn)
	for {
		select {
		case <-ctx.Done():
			return
		case f, ok := <-frames:
			if !ok {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(s.cfg.WriteTimeout))
			if err := w.WriteFrame(ctx, f); err != nil {
				return
			}
		}
	}
}

// ClientCount returns the current number of connected clients.
func (s *TCPServer) ClientCount() int64 { return s.clients.Load() }
