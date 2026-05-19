package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrameStorageWritesArchivedAndCurrentFrame(t *testing.T) {
	dir := t.TempDir()
	framesDir := filepath.Join(dir, "frames")
	currentPath := filepath.Join(dir, "current-image.jpeg")
	payload := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}

	storage, err := newFrameStorage(framesDir, currentPath)
	if err != nil {
		t.Fatalf("newFrameStorage returned error: %v", err)
	}

	archivedPath, err := storage.writeFrame(frameHeader{
		Sequence:    7,
		PayloadSize: uint32(len(payload)),
		TimestampMs: 1234,
	}, payload)
	if err != nil {
		t.Fatalf("writeFrame returned error: %v", err)
	}

	archived, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("read archived frame: %v", err)
	}

	current, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatalf("read current frame: %v", err)
	}

	if string(archived) != string(payload) {
		t.Fatalf("archived payload = %v, want %v", archived, payload)
	}
	if string(current) != string(payload) {
		t.Fatalf("current payload = %v, want %v", current, payload)
	}
}
