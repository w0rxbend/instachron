package app

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultAddr         = ":8080"
	defaultWebDir       = "./web"
	defaultSocketPath   = "/tmp/instachron/frames.sock"
	defaultCameraConfig = "./cameras.json"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	webDir := envString("WEB_DIR", defaultWebDir)
	socketPath := envString("IPC_SOCKET_PATH", defaultSocketPath)

	indexHTML, err := os.ReadFile(webDir + "/index.html")
	if err != nil {
		logger.Fatalf("load index.html from %s: %v", webDir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rotation, err := loadRotationConfig(envString("CAMERA_CONFIG", defaultCameraConfig), logger)
	if err != nil {
		logger.Fatalf("load rotation config: %v", err)
	}

	manager := newHubManager(ctx, rotation)
	go manager.run()

	reader := newSocketReader(socketPath, manager, logger)
	go reader.run(ctx)

	api := &apiServer{manager: manager, indexHTML: indexHTML, logger: logger}
	httpSrv := &http.Server{
		Addr:        addr,
		Handler:     api.routes(),
		ReadTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Printf("HTTP shutdown error: %v", err)
		}
	}()

	logger.Printf("camera-web-api listening on %s  ipc=%s", addr, socketPath)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("HTTP server failed: %v", err)
	}
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
