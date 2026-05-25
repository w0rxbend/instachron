// Package rotation provides JPEG rotation support driven by a JSON config file.
package rotation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
)

// Config maps camera ID strings to clockwise rotation angles (0/90/180/270).
// The file format is a JSON object: {"0": 90, "1": -90, "2": 180}.
// Any multiple of 90 is accepted; values are normalised to [0, 270].
type Config struct {
	mu     sync.RWMutex
	angles map[string]int
	path   string
}

// NewEmpty returns a no-op Config with no rotations configured.
func NewEmpty() *Config {
	return &Config{angles: make(map[string]int)}
}

// Load reads a JSON file at path and returns the parsed Config.
// A missing file is not an error — it returns an empty (no-op) config.
func Load(path string, logger interface{ Printf(string, ...any) }) (*Config, error) {
	rc := &Config{angles: make(map[string]int), path: path}
	if err := rc.Reload(logger); err != nil {
		return nil, err
	}
	return rc, nil
}

// Reload re-reads the config file. Safe to call concurrently.
func (rc *Config) Reload(logger interface{ Printf(string, ...any) }) error {
	data, err := os.ReadFile(rc.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", rc.path, err)
	}

	var raw map[string]int
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse %s: %w", rc.path, err)
	}

	angles := make(map[string]int, len(raw))
	for id, deg := range raw {
		angles[id] = normalizeDeg(deg)
	}

	rc.mu.Lock()
	rc.angles = angles
	rc.mu.Unlock()

	logger.Printf("rotation config loaded from %s: %d entries", rc.path, len(angles))
	return nil
}

// Get returns the configured clockwise rotation (0/90/180/270) for a camera ID.
func (rc *Config) Get(cameraID string) int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.angles[cameraID]
}

// normalizeDeg converts any integer angle to the nearest multiple of 90 in [0, 270].
func normalizeDeg(deg int) int {
	n := ((deg % 360) + 360) % 360
	switch {
	case n < 45 || n >= 315:
		return 0
	case n < 135:
		return 90
	case n < 225:
		return 180
	default:
		return 270
	}
}

// Apply decodes the JPEG, rotates by the configured angle, and re-encodes.
// Returns the original bytes unchanged if degrees == 0 or on any error.
func Apply(data []byte, degrees int) []byte {
	if degrees == 0 {
		return data
	}

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	var rotated image.Image
	switch degrees {
	case 90:
		rotated = rotate90(img)
	case 180:
		rotated = rotate180(img)
	case 270:
		rotated = rotate270(img)
	default:
		return data
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rotated, &jpeg.Options{Quality: 90}); err != nil {
		return data
	}
	return buf.Bytes()
}

func rotate90(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
