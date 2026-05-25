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
	"github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api/internal/enhance"
)

const (
	defaultAddr         = ":8091"
	defaultUpstreamAddr = "localhost:9001"
	defaultTCPAddr      = ":9003"
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	upstreamTCPAddr := envString("UPSTREAM_TCP_ADDR", defaultUpstreamAddr)
	tcpAddr := envString("TCP_ADDR", defaultTCPAddr)
	tcpEnabled := envString("TCP_ENABLED", "true") != "false"

	configPath := envString("CONFIG_FILE", "config.json")
	cameraCfgs, err := enhance.LoadCameraConfigs(configPath)
	if err != nil {
		logger.Printf("config file %q not found (%v), using built-in defaults", configPath, err)
		cameraCfgs = enhance.DefaultCameraConfigs()
	} else {
		logger.Printf("loaded enhancer config from %s (%d camera overrides)", configPath, len(cameraCfgs.Cameras))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := restream.NewManager()
	proc := enhance.New(cameraCfgs)

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

	// broadcaster fans enhanced frames to downstream TCP proxy clients
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
			proc.ProcessCamera(id, f.Payload, func(processed []byte) {
				pf := streamproto.Frame{
					CameraID:  f.CameraID,
					Timestamp: f.Timestamp,
					Sequence:  f.Sequence,
					Payload:   processed,
				}
				broadcaster.Publish(pf)
				manager.Push(id, processed)
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

	logger.Printf("camera-web-restream-enhancer-api listening on %s  upstream=%s  tcp=%s (enabled=%v)",
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

