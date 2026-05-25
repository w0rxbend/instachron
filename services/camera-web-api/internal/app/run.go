package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/w0rxbend/instachron/services/camera-web-api/internal/camera"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/httpapi"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/ipcclient"
	"github.com/w0rxbend/instachron/services/camera-web-api/internal/rotation"
	"github.com/w0rxbend/instachron/shared/restream"
	"github.com/w0rxbend/instachron/shared/streamproto"
)

const (
	defaultAddr         = ":8080"
	defaultWebDir       = "./web"
	defaultSocketPath   = "/tmp/instachron/frames.sock"
	defaultCameraConfig = "./cameras.json"
	defaultTCPAddr      = ":9001"
	defaultMaxClients   = 64
)

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	addr := envString("HTTP_ADDR", defaultAddr)
	webDir := envString("WEB_DIR", defaultWebDir)
	socketPath := envString("IPC_SOCKET_PATH", defaultSocketPath)
	cameraConfigPath := envString("CAMERA_CONFIG", defaultCameraConfig)
	tcpAddr := envString("TCP_ADDR", defaultTCPAddr)
	tcpEnabled := envString("TCP_ENABLED", "true") != "false"

	indexHTML, err := os.ReadFile(filepath.Join(webDir, "index.html"))
	if err != nil {
		logger.Fatalf("load index.html from %s: %v", webDir, err)
	}

	rotCfg, err := rotation.Load(cameraConfigPath, logger)
	if err != nil {
		logger.Fatalf("load rotation config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := camera.NewManager()

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

	// broadcaster fans frames out to TCP downstream clients (proxy-to-proxy transport)
	broadcaster := restream.NewBroadcaster()

	// per-camera sequence counters; only written by the single IPC reader goroutine
	seqs := make(map[uint32]uint64)

	reader := ipcclient.New(socketPath, ipcclient.Handler{
		OnFrame: func(cameraID uint32, jpeg []byte) {
			id := fmt.Sprintf("%d", cameraID)
			if deg := rotCfg.Get(id); deg != 0 {
				jpeg = rotation.Apply(jpeg, deg)
			}
			manager.Push(id, jpeg)

			seqs[cameraID]++
			broadcaster.Publish(streamproto.Frame{
				Timestamp: time.Now(),
				Sequence:  seqs[cameraID],
				CameraID:  cameraID,
				Payload:   jpeg,
			})
		},
		OnOffline: func(cameraID uint32) {
			manager.MarkOffline(fmt.Sprintf("%d", cameraID))
		},
		OnDisconnect: manager.MarkAllOffline,
	}, logger)
	go reader.Run(ctx)

	if tcpEnabled {
		tcpSrv := restream.NewTCPServer(restream.TCPServerConfig{
			ListenAddr:   tcpAddr,
			MaxClients:   defaultMaxClients,
			WriteTimeout: 2 * time.Second,
		}, broadcaster, logger)
		go func() {
			if err := tcpSrv.Run(ctx); err != nil {
				logger.Printf("TCP server error: %v", err)
			}
		}()
	}

	h := httpapi.New(manager, rotCfg, indexHTML, logger)
	httpSrv := &http.Server{
		Addr:        addr,
		Handler:     h.Routes(),
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

	logger.Printf("camera-web-api listening on %s  ipc=%s  tcp=%s (enabled=%v)",
		addr, socketPath, tcpAddr, tcpEnabled)
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
