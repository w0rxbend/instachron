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

	"github.com/w0rxbend/instachron/services/camera-recorder/internal/config"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/encoder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/recorder"
	"github.com/w0rxbend/instachron/services/camera-recorder/internal/storage"
	"github.com/w0rxbend/instachron/shared/restream"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	configPath := envString("CONFIG_FILE", config.DefaultPath)
	cfg, err := config.Load(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Printf("config file %q not found, using defaults and env overrides", configPath)
			cfg, err = config.Load("")
		}
		if err != nil {
			logger.Fatalf("config failed: %v", err)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := storage.NewLocal(cfg.Storage.RootDir)
	m := metrics.New()
	rec := recorder.NewManager(recorder.Config{
		OutputFPS:             cfg.Recording.OutputFPS,
		TimelapseFactor:       cfg.Recording.TimelapseFactor,
		SegmentRawDuration:    cfg.SegmentRawDuration(),
		MaxFileBytes:          cfg.Recording.MaxFileBytes,
		KeepFilesPerCamera:    cfg.Recording.KeepFilesPerCamera,
		QueueSizePerCamera:    cfg.Recording.QueueSizePerCamera,
		InactiveCloseDuration: cfg.InactiveCloseDuration(),
		FFmpeg: encoder.Config{
			Path:   cfg.FFmpeg.Path,
			Preset: cfg.FFmpeg.Preset,
			CRF:    cfg.FFmpeg.CRF,
		},
	}, store, m, logger)
	defer rec.Close()

	go refreshUsage(ctx, store, m, logger)

	upstream := restream.NewTCPUpstream(
		restream.TCPUpstreamConfig{Addr: cfg.UpstreamTCPAddr},
		func(f streamproto.Frame) {
			rec.Submit(ctx, f)
		},
		nil,
		logger,
	)
	go upstream.Run(ctx)

	api := newAPI(store, rec, m, logger)
	httpSrv := &http.Server{
		Addr:        cfg.HTTPAddr,
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

	logger.Printf("camera-recorder listening on %s upstream=%s storage=%s timelapse=%dx output_fps=%d",
		cfg.HTTPAddr, cfg.UpstreamTCPAddr, cfg.Storage.RootDir, cfg.Recording.TimelapseFactor, cfg.Recording.OutputFPS)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("HTTP server failed: %v", err)
	}
}

func refreshUsage(ctx context.Context, store storage.Store, m *metrics.Metrics, logger *log.Logger) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if n, err := store.UsageBytes(ctx); err == nil {
			m.SetStorageBytes(n)
		} else if ctx.Err() == nil {
			logger.Printf("storage usage: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
