package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	ort "github.com/yalue/onnxruntime_go"

	"github.com/w0rxbend/instachron/shared/restream"
	"github.com/w0rxbend/instachron/shared/streamproto"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/fsrcnn"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/metrics"
	"github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api/internal/pipeline"
)

type appConfig struct {
	httpAddr        string
	upstreamTCPAddr string
	tcpAddr         string
	tcpEnabled      bool
	modelPath       string
	onnxLibPath     string
	inputName       string
	outputName      string
	scale           int
	numWorkers      int
	intraOpThreads  int
	interOpThreads  int
	queueMultiplier int
	maxInputWidth   int
	maxInputHeight  int
	jpegQuality     int
	warmupRuns      int
	metricsInterval time.Duration
}

func loadConfig() *appConfig {
	numCPU := runtime.NumCPU()
	intraOp := envInt("INTRA_OP_THREADS", 2)
	workers := envInt("NUM_WORKERS", max(1, numCPU/intraOp))

	return &appConfig{
		httpAddr:        envStr("HTTP_ADDR", ":8092"),
		upstreamTCPAddr: envStr("UPSTREAM_TCP_ADDR", "localhost:9001"),
		tcpAddr:         envStr("TCP_ADDR", ":9004"),
		tcpEnabled:      envStr("TCP_ENABLED", "true") != "false",
		modelPath:       envStr("MODEL_PATH", "./models/fsrcnn_x2.onnx"),
		onnxLibPath:     envStr("ONNX_LIB_PATH", ""),
		inputName:       envStr("ONNX_INPUT_NAME", "input"),
		outputName:      envStr("ONNX_OUTPUT_NAME", "output"),
		scale:           envInt("SCALE_FACTOR", 2),
		numWorkers:      workers,
		intraOpThreads:  intraOp,
		interOpThreads:  envInt("INTER_OP_THREADS", 1),
		queueMultiplier: envInt("QUEUE_MULTIPLIER", 2),
		maxInputWidth:   envInt("MAX_INPUT_WIDTH", 960),
		maxInputHeight:  envInt("MAX_INPUT_HEIGHT", 540),
		jpegQuality:     envInt("JPEG_QUALITY", 85),
		warmupRuns:      envInt("WARMUP_RUNS", 5),
		metricsInterval: time.Duration(envInt("METRICS_INTERVAL_SEC", 60)) * time.Second,
	}
}

func Run() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	cfg := loadConfig()

	logger.Printf("camera-web-restream-fsrcnn-api config: workers=%d intraOpThreads=%d scale=%d queue=%d max=%dx%d model=%s",
		cfg.numWorkers, cfg.intraOpThreads, cfg.scale,
		cfg.numWorkers*cfg.queueMultiplier,
		cfg.maxInputWidth, cfg.maxInputHeight,
		cfg.modelPath)

	if cfg.onnxLibPath != "" {
		ort.SetSharedLibraryPath(cfg.onnxLibPath)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		logger.Fatalf("ort init: %v", err)
	}
	defer ort.DestroyEnvironment()

	sessions := make([]*fsrcnn.Session, cfg.numWorkers)
	for i := range sessions {
		sess, err := fsrcnn.New(
			cfg.modelPath,
			cfg.inputName,
			cfg.outputName,
			cfg.scale,
			cfg.intraOpThreads,
			cfg.interOpThreads,
		)
		if err != nil {
			logger.Fatalf("create session %d: %v\n\nhint: place the ONNX model at %s\nhint: set ONNX_LIB_PATH to the path of libonnxruntime.so if ORT is not on LD_LIBRARY_PATH",
				i, err, cfg.modelPath)
		}
		sessions[i] = sess
	}

	if cfg.warmupRuns > 0 {
		logger.Printf("warming up %d session(s) with %d runs each...", cfg.numWorkers, cfg.warmupRuns)
		pipeline.Warmup(sessions, cfg.scale, cfg.warmupRuns)
		logger.Printf("warmup complete")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	m := &metrics.Pipeline{}
	pool := pipeline.New(
		sessions,
		cfg.numWorkers*cfg.queueMultiplier,
		cfg.jpegQuality,
		cfg.maxInputWidth,
		cfg.maxInputHeight,
		cfg.scale,
		m,
	)
	defer pool.Close()

	go m.RunReporter(ctx, cfg.metricsInterval, logger)

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

	// broadcaster fans upscaled frames to downstream TCP proxy clients
	broadcaster := restream.NewBroadcaster()

	if cfg.tcpEnabled {
		tcpSrv := restream.NewTCPServer(restream.TCPServerConfig{
			ListenAddr:   cfg.tcpAddr,
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
		restream.TCPUpstreamConfig{Addr: cfg.upstreamTCPAddr},
		func(f streamproto.Frame) {
			id := fmt.Sprintf("%d", f.CameraID)
			pool.Process(f.Payload, func(processed []byte) {
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
		Addr:        cfg.httpAddr,
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

	logger.Printf("camera-web-restream-fsrcnn-api listening on %s  upstream=%s  tcp=%s (enabled=%v)",
		cfg.httpAddr, cfg.upstreamTCPAddr, cfg.tcpAddr, cfg.tcpEnabled)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("HTTP server failed: %v", err)
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
