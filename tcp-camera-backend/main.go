package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

const (
	defaultAddr          = "0.0.0.0:5000"
	defaultOutputDir     = "./frames"
	defaultCurrentImage  = "./current-image.jpeg"
	defaultMaxFrameBytes = 5 * 1024 * 1024
	defaultReadTimeout   = 30 * time.Second
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	storage, err := newFrameStorage(
		envString("FRAME_OUTPUT_DIR", defaultOutputDir),
		envString("CURRENT_IMAGE_PATH", defaultCurrentImage),
	)
	if err != nil {
		logger.Fatalf("storage init failed: %v", err)
	}

	server := &tcpFrameServer{
		addr:          envString("TCP_ADDR", defaultAddr),
		maxFrameBytes: envUint32("MAX_FRAME_BYTES", defaultMaxFrameBytes),
		readTimeout:   envDuration("READ_TIMEOUT", defaultReadTimeout),
		storage:       storage,
		logger:        logger,
	}

	if err := server.listenAndServe(); err != nil {
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
