package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type frameStorage struct {
	dir string
}

func newFrameStorage(dir string) (*frameStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create frame output directory: %w", err)
	}

	return &frameStorage{dir: dir}, nil
}

func (s *frameStorage) writeFrame(frame frameHeader, payload []byte) (string, error) {
	cameraDir := filepath.Join(s.dir, cameraDirName(frame.CameraID))
	filename := fmt.Sprintf("frame_%010d_%010d.jpg", frame.Sequence, frame.TimestampMs)
	path := filepath.Join(cameraDir, filename)

	if err := writeFileAtomic(path, payload); err != nil {
		return "", err
	}

	if err := writeFileAtomic(filepath.Join(cameraDir, "current-image.jpeg"), payload); err != nil {
		return "", err
	}

	pruneOldFrames(cameraDir, filename)
	return path, nil
}

// pruneOldFrames deletes all archived frame files in cameraDir except keep.
// current-image.jpeg and non-frame files are left untouched.
func pruneOldFrames(cameraDir, keep string) {
	entries, err := os.ReadDir(cameraDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == keep || e.Name() == "current-image.jpeg" {
			continue
		}
		if isArchivedFrame(e.Name()) {
			_ = os.Remove(filepath.Join(cameraDir, e.Name()))
		}
	}
}

func isArchivedFrame(name string) bool {
	return strings.HasPrefix(name, "frame_") && strings.HasSuffix(name, ".jpg")
}

func cameraDirName(cameraID uint32) string {
	return strconv.FormatUint(uint64(cameraID), 10)
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(dir, ".tmp-frame-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	cleanup = false
	return nil
}
