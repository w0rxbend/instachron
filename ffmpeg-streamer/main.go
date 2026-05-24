package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultSocketPath    = "/tmp/instachron/frames.sock"
	defaultFFmpegPath    = "ffmpeg"
	defaultFrameRate     = 10
	defaultRestartDelay  = 5 * time.Second
	defaultCameraID      = 0
	defaultCellWidth     = 320
	defaultCellHeight    = 240
)

type config struct {
	socketPath   string
	cameraID     uint32
	ffmpegPath   string
	streamURL    string
	frameRate    int
	restartDelay time.Duration
	mergeAll     bool
	cellWidth    int
	cellHeight   int
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	cfg, err := loadConfig(os.Args[1:])
	if err != nil {
		logger.Fatalf("config failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ipc := newIPCReader(cfg.socketPath, logger)
	go ipc.run(ctx)

	if err := run(ctx, cfg, ipc, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("streamer failed: %v", err)
	}
}

func loadConfig(args []string) (config, error) {
	streamURL, err := streamURLFromEnv()
	if err != nil {
		return config{}, err
	}

	cfg := config{
		socketPath:   envString("IPC_SOCKET_PATH", defaultSocketPath),
		cameraID:     envUint32("CAMERA_ID", defaultCameraID),
		ffmpegPath:   envString("FFMPEG_PATH", defaultFFmpegPath),
		streamURL:    streamURL,
		frameRate:    envInt("STREAM_FRAME_RATE", defaultFrameRate),
		restartDelay: envDuration("FFMPEG_RESTART_DELAY", defaultRestartDelay),
		mergeAll:     envBool("MERGE_ALL", false),
		cellWidth:    envInt("CELL_WIDTH", defaultCellWidth),
		cellHeight:   envInt("CELL_HEIGHT", defaultCellHeight),
	}

	flags := flag.NewFlagSet("ffmpeg-streamer", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Func("camera-id", "camera id to stream", func(value string) error {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid camera id %q: %w", value, err)
		}
		cfg.cameraID = uint32(parsed)
		return nil
	})
	flags.BoolVar(&cfg.mergeAll, "merge", cfg.mergeAll, "merge all cameras into a single canvas")
	flags.IntVar(&cfg.cellWidth, "cell-width", cfg.cellWidth, "cell width per camera in merged canvas")
	flags.IntVar(&cfg.cellHeight, "cell-height", cfg.cellHeight, "cell height per camera in merged canvas")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}

	if cfg.frameRate <= 0 {
		return config{}, fmt.Errorf("STREAM_FRAME_RATE must be greater than 0")
	}
	if cfg.restartDelay <= 0 {
		return config{}, fmt.Errorf("FFMPEG_RESTART_DELAY must be greater than 0")
	}
	if cfg.cellWidth <= 0 {
		return config{}, fmt.Errorf("CELL_WIDTH must be greater than 0")
	}
	if cfg.cellHeight <= 0 {
		return config{}, fmt.Errorf("CELL_HEIGHT must be greater than 0")
	}
	if cfg.cellWidth%2 != 0 {
		cfg.cellWidth++
	}
	if cfg.cellHeight%2 != 0 {
		cfg.cellHeight++
	}

	return cfg, nil
}

func streamURLFromEnv() (string, error) {
	if direct := envString("STREAM_URL", ""); direct != "" {
		return direct, nil
	}
	if direct := envString("RTMP_URL", ""); direct != "" {
		return direct, nil
	}

	twitchKey := envString("TWITCH_STREAM_KEY", "")
	youtubeKey := envString("YOUTUBE_STREAM_KEY", "")

	switch {
	case twitchKey != "" && youtubeKey != "":
		return "", fmt.Errorf("set only one of TWITCH_STREAM_KEY or YOUTUBE_STREAM_KEY")
	case twitchKey != "":
		return "rtmp://live.twitch.tv/app/" + twitchKey, nil
	case youtubeKey != "":
		return "rtmp://a.rtmp.youtube.com/live2/" + youtubeKey, nil
	default:
		return "", fmt.Errorf("set STREAM_URL, RTMP_URL, TWITCH_STREAM_KEY, or YOUTUBE_STREAM_KEY")
	}
}

func run(ctx context.Context, cfg config, ipc *ipcReader, logger *log.Logger) error {
	if cfg.mergeAll {
		logger.Printf("merging all cameras via IPC %s at %d fps", cfg.socketPath, cfg.frameRate)
	} else {
		logger.Printf("watching camera=%d via IPC %s at %d fps", cfg.cameraID, cfg.socketPath, cfg.frameRate)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := runFFmpegSession(ctx, cfg, ipc, logger)
		if errors.Is(err, context.Canceled) {
			return err
		}
		if err != nil {
			logger.Printf("ffmpeg session ended: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.restartDelay):
			logger.Printf("restarting ffmpeg")
		}
	}
}

func runFFmpegSession(ctx context.Context, cfg config, ipc *ipcReader, logger *log.Logger) error {
	args := ffmpegArgs(cfg)
	cmd := exec.CommandContext(ctx, cfg.ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("open ffmpeg stdin: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	var feedErr error
	if cfg.mergeAll {
		feedErr = feedMergedFrames(ctx, cfg, ipc, stdin, logger)
	} else {
		feedErr = feedFrames(ctx, cfg, ipc, stdin, logger)
	}
	_ = stdin.Close()

	waitErr := cmd.Wait()
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	if feedErr != nil {
		return feedErr
	}
	if waitErr != nil {
		return fmt.Errorf("ffmpeg exited: %w", waitErr)
	}

	return nil
}

func ffmpegArgs(cfg config) []string {
	gop := cfg.frameRate * 2
	return []string{
		"-hide_banner",
		"-loglevel", "warning",
		"-f", "mjpeg",
		"-framerate", strconv.Itoa(cfg.frameRate),
		"-i", "pipe:0",
		"-an",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-r", strconv.Itoa(cfg.frameRate),
		"-g", strconv.Itoa(gop),
		"-f", "flv",
		cfg.streamURL,
	}
}

// feedFrames reads the latest JPEG for the configured camera from the IPC reader
// and feeds it to ffmpeg at the configured frame rate.
func feedFrames(ctx context.Context, cfg config, ipc *ipcReader, writer io.Writer, logger *log.Logger) error {
	frameInterval := time.Second / time.Duration(cfg.frameRate)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			f := ipc.latest(cfg.cameraID)
			if len(f) == 0 {
				continue
			}
			if _, err := writer.Write(f); err != nil {
				return fmt.Errorf("write frame to ffmpeg: %w", err)
			}
		}
	}
}

func looksLikeJPEG(payload []byte) bool {
	if len(payload) < 4 {
		return false
	}
	return payload[0] == 0xFF &&
		payload[1] == 0xD8 &&
		payload[len(payload)-2] == 0xFF &&
		payload[len(payload)-1] == 0xD9
}

func envString(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envUint32(key string, fallback uint32) uint32 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseUint(value, 10, 32); err == nil {
			return uint32(parsed)
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	switch os.Getenv(key) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	}
	return fallback
}
