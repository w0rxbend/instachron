package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindLatestJPEG(t *testing.T) {
	dir := t.TempDir()

	oldPath := filepath.Join(dir, "frame_0000000001_0000000001.jpg")
	newPath := filepath.Join(dir, "frame_0000000002_0000000002.jpeg")
	ignoredPath := filepath.Join(dir, "notes.txt")

	if err := os.WriteFile(oldPath, []byte{0xFF, 0xD8, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatalf("write old frame: %v", err)
	}
	if err := os.WriteFile(newPath, []byte{0xFF, 0xD8, 0xAA, 0xFF, 0xD9}, 0o644); err != nil {
		t.Fatalf("write new frame: %v", err)
	}
	if err := os.WriteFile(ignoredPath, []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	oldTime := time.Now().Add(-time.Minute)
	newTime := time.Now()
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old frame: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new frame: %v", err)
	}

	got, found, err := findLatestJPEG(dir)
	if err != nil {
		t.Fatalf("findLatestJPEG returned error: %v", err)
	}
	if !found {
		t.Fatal("findLatestJPEG did not find a frame")
	}
	if got.path != newPath {
		t.Fatalf("latest path = %s, want %s", got.path, newPath)
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
