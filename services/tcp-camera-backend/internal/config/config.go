package config

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultAddr          = "0.0.0.0:5000"
	defaultSocketPath    = "/tmp/instachron/frames.sock"
	defaultMaxFrameBytes = 5 * 1024 * 1024
	defaultReadTimeout   = 30 * time.Second
)

type Config struct {
	TCPAddr       string
	IPCSocketPath string
	MaxFrameBytes uint32
	ReadTimeout   time.Duration
}

func LoadFromEnv() Config {
	return Config{
		TCPAddr:       envString("TCP_ADDR", defaultAddr),
		IPCSocketPath: envString("IPC_SOCKET_PATH", defaultSocketPath),
		MaxFrameBytes: envUint32("MAX_FRAME_BYTES", defaultMaxFrameBytes),
		ReadTimeout:   envDuration("READ_TIMEOUT", defaultReadTimeout),
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
