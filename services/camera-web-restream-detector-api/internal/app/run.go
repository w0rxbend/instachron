package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/w0rxbend/instachron/shared/restream"
	"github.com/w0rxbend/instachron/shared/streamproto"
	"github.com/w0rxbend/instachron/services/camera-web-restream-detector-api/internal/detect"
)

const (
	defaultAddr         = ":8093"
	defaultUpstreamAddr = "localhost:9001"
	defaultTCPAddr      = ":9005"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	upstreamTCPAddr := envString("UPSTREAM_TCP_ADDR", defaultUpstreamAddr)
	tcpAddr := envString("TCP_ADDR", defaultTCPAddr)
	tcpEnabled := envString("TCP_ENABLED", "true") != "false"
	configPath := envString("CONFIG_FILE", "config.json")

	cfg, err := detect.LoadConfig(configPath)
	if err != nil {
		logger.Printf("config file %q not found (%v), using built-in defaults", configPath, err)
		cfg = detect.DefaultConfig()
	} else {
		logger.Printf("loaded detector config from %s (model=%s conf=%.2f nms=%.2f)",
			configPath, cfg.ModelPath, cfg.ConfThreshold, cfg.NMSThreshold)
	}

	// ORT_LIB_PATH env var overrides whatever is in config.json, making it easy
	// to point at a locally downloaded libonnxruntime.so without editing the file.
	if v := envString("ORT_LIB_PATH", ""); v != "" {
		cfg.OrtLibPath = v
	}

	logger.Printf("ORT library path: %q (override with ORT_LIB_PATH env var)", cfg.OrtLibPath)

	// load the detector; fall back to passthrough if the model isn't available yet
	var processor restream.Processor
	det, err := detect.New(cfg, logger)
	if err != nil {
		logger.Printf("detector unavailable (%v) — frames will pass through unchanged", err)
		logger.Printf("hint: download ONNX Runtime from https://github.com/microsoft/onnxruntime/releases and set ORT_LIB_PATH=/path/to/libonnxruntime.so.x.y.z")
		processor = restream.Noop{}
	} else {
		logger.Printf("YOLOv8 detector ready (input %dx%d, %d classes, output %s)",
			cfg.InputWidth, cfg.InputHeight, cfg.NumClasses,
			func() string {
				if det.Layout().Transposed {
					return "transposed [1,boxes,channels]"
				}
				return "channel-first [1,channels,boxes]"
			}())
		processor = det
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if det != nil {
		go func() {
			<-ctx.Done()
			det.Destroy()
		}()
	}

	manager := restream.NewManager()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				manager.CheckLiveness()
			}
		}
	}()

	broadcaster := restream.NewBroadcaster()

	if tcpEnabled {
		tcpSrv := restream.NewTCPServer(restream.TCPServerConfig{
			ListenAddr:   tcpAddr,
			MaxClients:   64,
			WriteTimeout: 2 * time.Second,
		}, broadcaster, logger)
		go func() {
			if err := tcpSrv.Run(ctx); err != nil {
				logger.Printf("TCP server error: %v", err)
			}
		}()
	}

	upstream := restream.NewTCPUpstream(
		restream.TCPUpstreamConfig{Addr: upstreamTCPAddr},
		func(f streamproto.Frame) {
			id := fmt.Sprintf("%d", f.CameraID)
			processor.Process(f.Payload, func(annotated []byte) {
				pf := streamproto.Frame{
					CameraID:  f.CameraID,
					Timestamp: f.Timestamp,
					Sequence:  f.Sequence,
					Payload:   annotated,
				}
				broadcaster.Publish(pf)
				manager.Push(id, annotated)
			})
		},
		manager.MarkAllOffline,
		logger,
	)
	go upstream.Run(ctx)

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

	logger.Printf("camera-web-restream-detector-api listening on %s  upstream=%s  tcp=%s (enabled=%v)",
		addr, upstreamTCPAddr, tcpAddr, tcpEnabled)
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
