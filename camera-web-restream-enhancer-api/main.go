package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultAddr      = ":8091"
	defaultOriginURL = "http://localhost:8080"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr      := envString("HTTP_ADDR", defaultAddr)
	originURL := envString("ORIGIN_URL", defaultOriginURL)

	cfg := enhancerConfig{
		sharpen:       envFloat("SHARPEN", 1.0),
		darkThreshold: envFloat("DARK_THRESHOLD", 0.35),
		brightnessMax: envFloat("BRIGHTNESS_MAX", 30.0),
		contrastMax:   envFloat("CONTRAST_MAX", 25.0),
		jpegQuality:   envInt("JPEG_QUALITY", 85),
	}

	logger.Printf("enhancer config: sharpen=%.2f dark_threshold=%.2f brightness_max=%.1f contrast_max=%.1f jpeg_quality=%d",
		cfg.sharpen, cfg.darkThreshold, cfg.brightnessMax, cfg.contrastMax, cfg.jpegQuality)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := newHubManager()
	enh := newEnhancer(cfg)

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

	disc := newDiscovery(originURL, manager, enh, logger)
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

	logger.Printf("camera-web-restream-enhancer-api listening on %s  origin=%s", addr, originURL)
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

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
