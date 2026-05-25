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
	cameraConfigPath := envString("CAMERA_CONFIG", defaultCameraConfig)

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

	reader := ipcclient.New(socketPath, ipcclient.Handler{
		OnFrame: func(cameraID uint32, jpeg []byte) {
			id := fmt.Sprintf("%d", cameraID)
			if deg := rotCfg.Get(id); deg != 0 {
				jpeg = rotation.Apply(jpeg, deg)
			}
			manager.Push(id, jpeg)
		},
		OnOffline: func(cameraID uint32) {
			manager.MarkOffline(fmt.Sprintf("%d", cameraID))
		},
		OnDisconnect: manager.MarkAllOffline,
	}, logger)
	go reader.Run(ctx)

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
