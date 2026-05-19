package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type tcpFrameServer struct {
	addr          string
	maxFrameBytes uint32
	readTimeout   time.Duration
	storage       *frameStorage
	logger        *log.Logger
}

func (s *tcpFrameServer) listenAndServe() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	defer listener.Close()

	s.logger.Printf("TCP frame server listening on %s", listener.Addr())

	for {
		conn, err := listener.Accept()
		if err != nil {
			s.logger.Printf("accept failed: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *tcpFrameServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	remoteAddr := conn.RemoteAddr().String()
	s.logger.Printf("client connected: %s", remoteAddr)
	defer s.logger.Printf("client disconnected: %s", remoteAddr)

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

		header, err := parseFrameHeader(headerBytes)
		if err != nil {
			s.logger.Printf("bad header from %s: %v", remoteAddr, err)
			return
		}

		if header.PayloadSize == 0 || header.PayloadSize > s.maxFrameBytes {
			s.logger.Printf("invalid payload size from %s: seq=%d size=%d max=%d",
				remoteAddr, header.Sequence, header.PayloadSize, s.maxFrameBytes)
			return
		}

		payload := make([]byte, int(header.PayloadSize))
		if _, err := io.ReadFull(conn, payload); err != nil {
			s.logger.Printf("read payload from %s failed: seq=%d size=%d err=%v",
				remoteAddr, header.Sequence, header.PayloadSize, err)
			return
		}

		if !looksLikeJPEG(payload) {
			s.logger.Printf("dropping non-JPEG payload from %s: seq=%d size=%d",
				remoteAddr, header.Sequence, header.PayloadSize)
			continue
		}

		path, err := s.storage.writeFrame(header, payload)
		if err != nil {
			s.logger.Printf("write frame failed: seq=%d size=%d err=%v",
				header.Sequence, header.PayloadSize, err)
			return
		}

		s.logger.Printf("stored frame: seq=%d camera_ms=%d bytes=%d path=%s",
			header.Sequence, header.TimestampMs, header.PayloadSize, path)
	}
}
