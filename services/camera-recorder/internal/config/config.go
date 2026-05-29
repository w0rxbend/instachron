package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

const DefaultPath = "config.json"

type Config struct {
	HTTPAddr        string          `json:"http_addr"`
	UpstreamTCPAddr string          `json:"upstream_tcp_addr"`
	Recording       RecordingConfig `json:"recording"`
	Storage         StorageConfig   `json:"storage"`
	FFmpeg          FFmpegConfig    `json:"ffmpeg"`
}

type RecordingConfig struct {
	OutputFPS             int    `json:"output_fps"`
	TimelapseFactor       int    `json:"timelapse_factor"`
	SegmentRawDuration    string `json:"segment_raw_duration"`
	MaxFileBytes          int64  `json:"max_file_bytes"`
	KeepFilesPerCamera    int    `json:"keep_files_per_camera"`
	QueueSizePerCamera    int    `json:"queue_size_per_camera"`
	InactiveCloseDuration string `json:"inactive_close_duration"`
}

type StorageConfig struct {
	Type    string `json:"type"`
	RootDir string `json:"root_dir"`
}

type FFmpegConfig struct {
	Path   string `json:"path"`
	Preset string `json:"preset"`
	CRF    int    `json:"crf"`
}

func Defaults() Config {
	return Config{
		HTTPAddr:        ":8094",
		UpstreamTCPAddr: "localhost:9001",
		Recording: RecordingConfig{
			OutputFPS:             10,
			TimelapseFactor:       10,
			SegmentRawDuration:    "10m",
			MaxFileBytes:          100 * 1024 * 1024,
			KeepFilesPerCamera:    144,
			QueueSizePerCamera:    128,
			InactiveCloseDuration: "30s",
		},
		Storage: StorageConfig{
			Type:    "local",
			RootDir: "./recordings",
		},
		FFmpeg: FFmpegConfig{
			Path:   "ffmpeg",
			Preset: "veryfast",
			CRF:    23,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, err
		}
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) SegmentRawDuration() time.Duration {
	d, _ := time.ParseDuration(c.Recording.SegmentRawDuration)
	return d
}

func (c Config) InactiveCloseDuration() time.Duration {
	d, _ := time.ParseDuration(c.Recording.InactiveCloseDuration)
	return d
}

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("http_addr is required")
	}
	if c.UpstreamTCPAddr == "" {
		return fmt.Errorf("upstream_tcp_addr is required")
	}
	if c.Recording.OutputFPS <= 0 {
		return fmt.Errorf("recording.output_fps must be greater than 0")
	}
	if c.Recording.TimelapseFactor <= 0 {
		return fmt.Errorf("recording.timelapse_factor must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Recording.SegmentRawDuration); err != nil {
		return fmt.Errorf("recording.segment_raw_duration: %w", err)
	}
	if c.Recording.MaxFileBytes <= 0 {
		return fmt.Errorf("recording.max_file_bytes must be greater than 0")
	}
	if c.Recording.KeepFilesPerCamera <= 0 {
		return fmt.Errorf("recording.keep_files_per_camera must be greater than 0")
	}
	if c.Recording.QueueSizePerCamera <= 0 {
		return fmt.Errorf("recording.queue_size_per_camera must be greater than 0")
	}
	if _, err := time.ParseDuration(c.Recording.InactiveCloseDuration); err != nil {
		return fmt.Errorf("recording.inactive_close_duration: %w", err)
	}
	if c.Storage.Type != "local" {
		return fmt.Errorf("storage.type %q is unsupported", c.Storage.Type)
	}
	if c.Storage.RootDir == "" {
		return fmt.Errorf("storage.root_dir is required")
	}
	if c.FFmpeg.Path == "" {
		return fmt.Errorf("ffmpeg.path is required")
	}
	if c.FFmpeg.Preset == "" {
		return fmt.Errorf("ffmpeg.preset is required")
	}
	if c.FFmpeg.CRF < 0 || c.FFmpeg.CRF > 51 {
		return fmt.Errorf("ffmpeg.crf must be between 0 and 51")
	}
	return nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		c.HTTPAddr = v
	}
	if v := os.Getenv("UPSTREAM_TCP_ADDR"); v != "" {
		c.UpstreamTCPAddr = v
	}
	if v := os.Getenv("STORAGE_ROOT_DIR"); v != "" {
		c.Storage.RootDir = v
	}
	if v := os.Getenv("FFMPEG_PATH"); v != "" {
		c.FFmpeg.Path = v
	}
	if v := os.Getenv("FFMPEG_PRESET"); v != "" {
		c.FFmpeg.Preset = v
	}
	envInt("OUTPUT_FPS", &c.Recording.OutputFPS)
	envInt("TIMELAPSE_FACTOR", &c.Recording.TimelapseFactor)
	envInt64("MAX_FILE_BYTES", &c.Recording.MaxFileBytes)
	envInt("KEEP_FILES_PER_CAMERA", &c.Recording.KeepFilesPerCamera)
	envInt("QUEUE_SIZE_PER_CAMERA", &c.Recording.QueueSizePerCamera)
	envInt("FFMPEG_CRF", &c.FFmpeg.CRF)
	if v := os.Getenv("SEGMENT_RAW_DURATION"); v != "" {
		c.Recording.SegmentRawDuration = v
	}
	if v := os.Getenv("INACTIVE_CLOSE_DURATION"); v != "" {
		c.Recording.InactiveCloseDuration = v
	}
}

func envInt(key string, target *int) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*target = n
		}
	}
}

func envInt64(key string, target *int64) {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			*target = n
		}
	}
}
