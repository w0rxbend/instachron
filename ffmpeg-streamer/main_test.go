package main

import (
	"path/filepath"
	"testing"
)

func TestCameraFrameDirUsesCameraID(t *testing.T) {
	got := cameraFrameDir("/tmp/frames", 17)
	want := filepath.Join("/tmp/frames", "17")

	if got != want {
		t.Fatalf("camera frame dir = %s, want %s", got, want)
	}
}

func TestLoadConfigParsesCameraIDFlag(t *testing.T) {
	t.Setenv("STREAM_URL", "rtmp://example/live/key")
	t.Setenv("RTMP_URL", "")
	t.Setenv("TWITCH_STREAM_KEY", "")
	t.Setenv("YOUTUBE_STREAM_KEY", "")

	got, err := loadConfig([]string{"--camera-id", "42"})
	if err != nil {
		t.Fatalf("loadConfig returned error: %v", err)
	}

	if got.cameraID != 42 {
		t.Fatalf("cameraID = %d, want 42", got.cameraID)
	}
}

func TestStreamURLFromEnv(t *testing.T) {
	t.Setenv("STREAM_URL", "")
	t.Setenv("RTMP_URL", "")
	t.Setenv("TWITCH_STREAM_KEY", "twitch-key")
	t.Setenv("YOUTUBE_STREAM_KEY", "")

	got, err := streamURLFromEnv()
	if err != nil {
		t.Fatalf("streamURLFromEnv returned error: %v", err)
	}

	want := "rtmp://live.twitch.tv/app/twitch-key"
	if got != want {
		t.Fatalf("stream URL = %s, want %s", got, want)
	}
}

func TestLooksLikeJPEG(t *testing.T) {
	if !looksLikeJPEG([]byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}) {
		t.Fatal("looksLikeJPEG returned false for valid JPEG markers")
	}

	if looksLikeJPEG([]byte{0x00, 0xD8, 0xAA, 0xFF, 0x00}) {
		t.Fatal("looksLikeJPEG returned true for invalid JPEG markers")
	}
}
