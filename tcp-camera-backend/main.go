package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultAddr          = "0.0.0.0:5000"
	defaultSocketPath    = "/tmp/instachron/frames.sock"
	defaultMaxFrameBytes = 5 * 1024 * 1024
	defaultReadTimeout   = 30 * time.Second
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	socketPath := envString("IPC_SOCKET_PATH", defaultSocketPath)
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		logger.Fatalf("create IPC socket directory: %v", err)
	}
	publisher := newFramePublisher(socketPath, logger)

	server := &tcpFrameServer{
		addr:          envString("TCP_ADDR", defaultAddr),
		maxFrameBytes: envUint32("MAX_FRAME_BYTES", defaultMaxFrameBytes),
		readTimeout:   envDuration("READ_TIMEOUT", defaultReadTimeout),
		publisher:     publisher,
		logger:        logger,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := publisher.listen(ctx); err != nil {
			logger.Printf("IPC publisher error: %v", err)
		}
	}()

	if err := server.listenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("server failed: %v", err)
	}
}

func envString(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envUint32(key string, fallback uint32) uint32 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fallback
	}

	return uint32(parsed)
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
