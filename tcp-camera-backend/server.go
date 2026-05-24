package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type tcpFrameServer struct {
	addr          string
	maxFrameBytes uint32
	readTimeout   time.Duration
	publisher     *framePublisher
	logger        *log.Logger
	mu            sync.Mutex
	conns         map[net.Conn]struct{}
}

func (s *tcpFrameServer) listenAndServe(ctx context.Context) error {
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

func (s *tcpFrameServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	s.logger.Printf("client connected: %s", remoteAddr)

	seenCameraIDs := make(map[uint32]struct{})
	defer func() {
		for id := range seenCameraIDs {
			s.logger.Printf("camera offline: camera=%d addr=%s", id, remoteAddr)
			if s.publisher != nil {
				s.publisher.publishOffline(id)
			}
		}
		s.logger.Printf("client disconnected: %s", remoteAddr)
	}()

	headerBytes := make([]byte, frameHeaderSize)

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

		if !looksLikeJPEG(payload) {
			s.logger.Printf("dropping non-JPEG payload from %s: camera=%d seq=%d size=%d",
				remoteAddr, header.CameraID, header.Sequence, header.PayloadSize)
			continue
		}

		seenCameraIDs[header.CameraID] = struct{}{}

		if s.publisher != nil {
			s.publisher.publish(header.CameraID, payload)
		}

		s.logger.Printf("published frame: camera=%d seq=%d camera_ms=%d bytes=%d",
			header.CameraID, header.Sequence, header.TimestampMs, header.PayloadSize)
	}
}

func (s *tcpFrameServer) readFrameHeader(conn net.Conn, headerBytes []byte) (frameHeader, error) {
	magic := binary.BigEndian.Uint32(headerBytes[0:4])
	switch magic {
	case frameMagicLegacy:
		return parseFrameHeader(headerBytes)
	case frameMagicWithDevice:
		cameraIDBytes := make([]byte, frameCameraIDSize)
		if _, err := io.ReadFull(conn, cameraIDBytes); err != nil {
			return frameHeader{}, fmt.Errorf("read camera id: %w", err)
		}
		return parseFrameHeaderWithCameraID(headerBytes, cameraIDBytes)
	default:
		return frameHeader{}, fmt.Errorf("invalid frame magic: 0x%08x", magic)
	}
}
