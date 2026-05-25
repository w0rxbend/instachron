package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/protocol"
)

const frameStatsInterval = 5 * time.Second

type Publisher interface {
	Publish(cameraID uint32, jpeg []byte)
	PublishOffline(cameraID uint32)
}

type Config struct {
	Addr          string
	MaxFrameBytes uint32
	ReadTimeout   time.Duration
	Publisher     Publisher
	Logger        *log.Logger
}

type Server struct {
	addr          string
	maxFrameBytes uint32
	readTimeout   time.Duration
	publisher     Publisher
	logger        *log.Logger
	mu            sync.Mutex
	conns         map[net.Conn]struct{}
}

func New(cfg Config) *Server {
	return &Server{
		addr:          cfg.Addr,
		maxFrameBytes: cfg.MaxFrameBytes,
		readTimeout:   cfg.ReadTimeout,
		publisher:     cfg.Publisher,
		logger:        cfg.Logger,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	defer listener.Close()

	s.conns = make(map[net.Conn]struct{})

	var wg sync.WaitGroup
	go func() {
		<-ctx.Done()
		listener.Close()
		s.mu.Lock()
		for c := range s.conns {
			c.Close()
		}
		s.mu.Unlock()
	}()

	s.logger.Printf("TCP frame server listening on %s", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return ctx.Err()
			}
			s.logger.Printf("accept failed: %v", err)
			continue
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		wg.Add(1)
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
				wg.Done()
			}()
			s.handleConnection(conn)
		}()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	s.logger.Printf("client connected: %s", remoteAddr)

	seenCameraIDs := make(map[uint32]struct{})
	defer func() {
		for id := range seenCameraIDs {
			s.logger.Printf("camera offline: camera=%d addr=%s", id, remoteAddr)
			if s.publisher != nil {
				s.publisher.PublishOffline(id)
			}
		}
		s.logger.Printf("client disconnected: %s", remoteAddr)
	}()

	headerBytes := make([]byte, protocol.HeaderSize)
	stats := newFrameStats(s.logger, remoteAddr, frameStatsInterval)
	defer stats.Stop()
	go stats.Run()

	for {
		if s.readTimeout > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.readTimeout))
		}

		if _, err := io.ReadFull(conn, headerBytes); err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				s.logger.Printf("read header from %s failed: %v", remoteAddr, err)
			}
			return
		}

		header, err := s.readFrameHeader(conn, headerBytes)
		if err != nil {
			s.logger.Printf("bad header from %s: %v", remoteAddr, err)
			return
		}

		if header.PayloadSize == 0 || header.PayloadSize > s.maxFrameBytes {
			s.logger.Printf("invalid payload size from %s: camera=%d seq=%d size=%d max=%d",
				remoteAddr, header.CameraID, header.Sequence, header.PayloadSize, s.maxFrameBytes)
			return
		}

		payload := make([]byte, int(header.PayloadSize))
		if _, err := io.ReadFull(conn, payload); err != nil {
			s.logger.Printf("read payload from %s failed: camera=%d seq=%d size=%d err=%v",
				remoteAddr, header.CameraID, header.Sequence, header.PayloadSize, err)
			return
		}

		if !protocol.LooksLikeJPEG(payload) {
			s.logger.Printf("dropping non-JPEG payload from %s: camera=%d seq=%d size=%d",
				remoteAddr, header.CameraID, header.Sequence, header.PayloadSize)
			continue
		}

		seenCameraIDs[header.CameraID] = struct{}{}

		if s.publisher != nil {
			s.publisher.Publish(header.CameraID, payload)
		}

		stats.Record(header.CameraID)
	}
}

func (s *Server) readFrameHeader(conn net.Conn, headerBytes []byte) (protocol.Header, error) {
	magic := binary.BigEndian.Uint32(headerBytes[0:4])
	switch magic {
	case protocol.MagicLegacy:
		return protocol.ParseLegacyHeader(headerBytes)
	case protocol.MagicWithDevice:
		cameraIDBytes := make([]byte, protocol.CameraIDSize)
		if _, err := io.ReadFull(conn, cameraIDBytes); err != nil {
			return protocol.Header{}, fmt.Errorf("read camera id: %w", err)
		}
		return protocol.ParseDeviceHeader(headerBytes, cameraIDBytes)
	default:
		return protocol.Header{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
}

type frameStats struct {
	logger   *log.Logger
	addr     string
	interval time.Duration
	done     chan struct{}
	once     sync.Once

	mu       sync.Mutex
	byCamera map[uint32]uint64
}

func newFrameStats(logger *log.Logger, addr string, interval time.Duration) *frameStats {
	return &frameStats{
		logger:   logger,
		addr:     addr,
		interval: interval,
		done:     make(chan struct{}),
		byCamera: make(map[uint32]uint64),
	}
}

func (s *frameStats) Record(cameraID uint32) {
	s.mu.Lock()
	s.byCamera[cameraID]++
	s.mu.Unlock()
}

func (s *frameStats) Run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.logAndReset()
		case <-s.done:
			return
		}
	}
}

func (s *frameStats) Stop() {
	s.once.Do(func() {
		close(s.done)
	})
}

func (s *frameStats) logAndReset() {
	s.mu.Lock()
	if len(s.byCamera) == 0 {
		s.mu.Unlock()
		return
	}

	counts := make(map[uint32]uint64, len(s.byCamera))
	var total uint64
	for cameraID, frames := range s.byCamera {
		counts[cameraID] = frames
		total += frames
	}
	clear(s.byCamera)
	s.mu.Unlock()

	s.logger.Printf("frame stats: addr=%s interval=%s frames=%d camera_frames=%s",
		s.addr, s.interval, total, formatCameraCounts(counts))
}

func formatCameraCounts(counts map[uint32]uint64) string {
	cameraIDs := make([]uint32, 0, len(counts))
	for cameraID := range counts {
		cameraIDs = append(cameraIDs, cameraID)
	}
	sort.Slice(cameraIDs, func(i, j int) bool {
		return cameraIDs[i] < cameraIDs[j]
	})

	parts := make([]string, 0, len(cameraIDs))
	for _, cameraID := range cameraIDs {
		parts = append(parts, fmt.Sprintf("camera=%d frames=%d", cameraID, counts[cameraID]))
	}
	return strings.Join(parts, ", ")
}
