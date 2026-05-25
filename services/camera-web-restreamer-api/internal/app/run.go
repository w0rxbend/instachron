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
	defaultAddr      = ":8090"
	defaultOriginURL = "http://localhost:8080"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	originURL := envString("ORIGIN_URL", defaultOriginURL)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := newHubManager()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				manager.checkLiveness()
			}
		}
	}()

	disc := newDiscovery(originURL, manager, logger)
	go disc.run(ctx)

	api := &apiServer{manager: manager, logger: logger}
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

	logger.Printf("camera-web-restreamer-api listening on %s  origin=%s", addr, originURL)
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
