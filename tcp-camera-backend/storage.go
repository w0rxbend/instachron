package main

import (
	"fmt"
	"os"
	"path/filepath"
)

type frameStorage struct {
	dir         string
	currentPath string
}

func newFrameStorage(dir string, currentPath string) (*frameStorage, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create frame output directory: %w", err)
	}

	if currentDir := filepath.Dir(currentPath); currentDir != "." {
		if err := os.MkdirAll(currentDir, 0o755); err != nil {
			return nil, fmt.Errorf("create current image directory: %w", err)
		}
	}

	return &frameStorage{dir: dir, currentPath: currentPath}, nil
}

func (s *frameStorage) writeFrame(frame frameHeader, payload []byte) (string, error) {
	filename := fmt.Sprintf("frame_%010d_%010d.jpg", frame.Sequence, frame.TimestampMs)
	path := filepath.Join(s.dir, filename)

	if err := writeFileAtomic(path, payload); err != nil {
		return "", err
	}

	if err := writeFileAtomic(s.currentPath, payload); err != nil {
		return "", err
	}

	return path, nil
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
