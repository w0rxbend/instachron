package main

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
	defaultFrameDir     = "./../frames"
	defaultWebDir       = "./web"
	defaultPollInterval = 250 * time.Millisecond
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	frameDir := envString("FRAME_OUTPUT_DIR", defaultFrameDir)
	webDir := envString("WEB_DIR", defaultWebDir)
	poll := envDuration("POLL_INTERVAL", defaultPollInterval)

	indexHTML, err := os.ReadFile(webDir + "/index.html")
	if err != nil {
		logger.Fatalf("load index.html from %s: %v", webDir, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := newHubManager(frameDir, poll, ctx)
	go manager.run()

	api := &apiServer{manager: manager, indexHTML: indexHTML, logger: logger}
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: api.routes(),
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutCtx); err != nil {
			logger.Printf("HTTP shutdown error: %v", err)
		}
	}()

	logger.Printf("camera-web-api listening on %s  frame-dir=%s  poll=%s", addr, frameDir, poll)
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

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
