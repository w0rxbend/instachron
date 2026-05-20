package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrameStorageWritesArchivedAndCurrentFrame(t *testing.T) {
	dir := t.TempDir()
	framesDir := filepath.Join(dir, "frames")
	payload := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}

	storage, err := newFrameStorage(framesDir)
	if err != nil {
		t.Fatalf("newFrameStorage returned error: %v", err)
	}

	archivedPath, err := storage.writeFrame(frameHeader{
		CameraID:    17,
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

	current, err := os.ReadFile(filepath.Join(framesDir, "17", "current-image.jpeg"))
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

func TestFrameStoragePrunesOldFrames(t *testing.T) {
	dir := t.TempDir()
	framesDir := filepath.Join(dir, "frames")
	payload := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}

	storage, err := newFrameStorage(framesDir)
	if err != nil {
		t.Fatalf("newFrameStorage returned error: %v", err)
	}

	firstPath, err := storage.writeFrame(frameHeader{
		CameraID: 0, Sequence: 1, PayloadSize: uint32(len(payload)), TimestampMs: 100,
	}, payload)
	if err != nil {
		t.Fatalf("first writeFrame returned error: %v", err)
	}

	secondPath, err := storage.writeFrame(frameHeader{
		CameraID: 0, Sequence: 2, PayloadSize: uint32(len(payload)), TimestampMs: 200,
	}, payload)
	if err != nil {
		t.Fatalf("second writeFrame returned error: %v", err)
	}

	if _, err := os.Stat(firstPath); !os.IsNotExist(err) {
		t.Error("expected first archived frame to be pruned, but it still exists")
	}

	if _, err := os.Stat(secondPath); err != nil {
		t.Errorf("expected second archived frame to exist: %v", err)
	}

	cameraDir := filepath.Join(framesDir, "0")
	if _, err := os.Stat(filepath.Join(cameraDir, "current-image.jpeg")); err != nil {
		t.Errorf("expected current-image.jpeg to exist: %v", err)
	}
}
