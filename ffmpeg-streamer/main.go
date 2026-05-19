package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	defaultFrameDir     = "./frames"
	defaultFFmpegPath   = "ffmpeg"
	defaultFrameRate    = 10
	defaultPollInterval = 250 * time.Millisecond
	defaultRestartDelay = 5 * time.Second
)

type config struct {
	frameDir     string
	ffmpegPath   string
	streamURL    string
	frameRate    int
	pollInterval time.Duration
	restartDelay time.Duration
}

type latestFrame struct {
	path    string
	modTime time.Time
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	cfg, err := loadConfig()
	if err != nil {
		logger.Fatalf("config failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("streamer failed: %v", err)
	}
}

func loadConfig() (config, error) {
	streamURL, err := streamURLFromEnv()
	if err != nil {
		return config{}, err
	}

	cfg := config{
		frameDir:     envString("FRAME_OUTPUT_DIR", defaultFrameDir),
		ffmpegPath:   envString("FFMPEG_PATH", defaultFFmpegPath),
		streamURL:    streamURL,
		frameRate:    envInt("STREAM_FRAME_RATE", defaultFrameRate),
		pollInterval: envDuration("FRAME_POLL_INTERVAL", defaultPollInterval),
		restartDelay: envDuration("FFMPEG_RESTART_DELAY", defaultRestartDelay),
	}

	if cfg.frameRate <= 0 {
		return config{}, fmt.Errorf("STREAM_FRAME_RATE must be greater than 0")
	}
	if cfg.pollInterval <= 0 {
		return config{}, fmt.Errorf("FRAME_POLL_INTERVAL must be greater than 0")
	}
	if cfg.restartDelay <= 0 {
		return config{}, fmt.Errorf("FFMPEG_RESTART_DELAY must be greater than 0")
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

func run(ctx context.Context, cfg config, logger *log.Logger) error {
	logger.Printf("watching %s and streaming at %d fps", cfg.frameDir, cfg.frameRate)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := runFFmpegSession(ctx, cfg, logger)
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

func runFFmpegSession(ctx context.Context, cfg config, logger *log.Logger) error {
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

	feedErr := feedFrames(ctx, cfg, stdin, logger)
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

func feedFrames(ctx context.Context, cfg config, writer io.Writer, logger *log.Logger) error {
	frameInterval := time.Second / time.Duration(cfg.frameRate)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	pollTicker := time.NewTicker(cfg.pollInterval)
	defer pollTicker.Stop()

	var current []byte
	var currentFrame latestFrame

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollTicker.C:
			next, found, err := findLatestJPEG(cfg.frameDir)
			if err != nil {
				logger.Printf("scan frames failed: %v", err)
				continue
			}
			if !found || sameFrame(currentFrame, next) {
				continue
			}

			payload, err := os.ReadFile(next.path)
			if err != nil {
				logger.Printf("read frame failed: path=%s err=%v", next.path, err)
				continue
			}
			if !looksLikeJPEG(payload) {
				logger.Printf("skipping non-JPEG frame: %s", next.path)
				continue
			}

			current = payload
			currentFrame = next
			logger.Printf("streaming latest frame: %s", next.path)
		case <-ticker.C:
			if len(current) == 0 {
				continue
			}
			if _, err := writer.Write(current); err != nil {
				return fmt.Errorf("write frame to ffmpeg: %w", err)
			}
		}
	}
}

func findLatestJPEG(dir string) (latestFrame, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return latestFrame{}, false, nil
		}
		return latestFrame{}, false, err
	}

	var latest latestFrame
	found := false

	for _, entry := range entries {
		if entry.IsDir() || !isJPEGName(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return latestFrame{}, false, err
		}

		candidate := latestFrame{
			path:    filepath.Join(dir, entry.Name()),
			modTime: info.ModTime(),
		}

		if !found || newerFrame(candidate, latest) {
			latest = candidate
			found = true
		}
	}

	return latest, found, nil
}

func newerFrame(candidate latestFrame, current latestFrame) bool {
	if !candidate.modTime.Equal(current.modTime) {
		return candidate.modTime.After(current.modTime)
	}
	return candidate.path > current.path
}

func sameFrame(a latestFrame, b latestFrame) bool {
	return a.path == b.path && a.modTime.Equal(b.modTime)
}

func isJPEGName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".jpg" || ext == ".jpeg"
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
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
