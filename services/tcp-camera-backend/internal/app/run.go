package app

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/config"
	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/publisher"
	"github.com/w0rxbend/instachron/services/tcp-camera-backend/internal/server"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	cfg := config.LoadFromEnv()

	if err := os.MkdirAll(filepath.Dir(cfg.IPCSocketPath), 0o755); err != nil {
		logger.Fatalf("create IPC socket directory: %v", err)
	}

	pub := publisher.New(cfg.IPCSocketPath, logger)
	srv := server.New(server.Config{
		Addr:          cfg.TCPAddr,
		MaxFrameBytes: cfg.MaxFrameBytes,
		ReadTimeout:   cfg.ReadTimeout,
		Publisher:     pub,
		Logger:        logger,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := pub.Listen(ctx); err != nil {
			logger.Printf("IPC publisher error: %v", err)
		}
	}()

	if err := srv.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("server failed: %v", err)
	}
}
