package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"sync"
)

// rotationConfig maps camera ID strings to clockwise rotation angles (0/90/180/270).
// The file format is a JSON object: {"0": 90, "1": -90, "2": 180}.
// Any multiple of 90 is accepted; values are normalised to [0, 270].
type rotationConfig struct {
	mu     sync.RWMutex
	angles map[string]int
	path   string
}

func newRotationConfig() *rotationConfig {
	return &rotationConfig{angles: make(map[string]int)}
}

// loadRotationConfig reads a JSON file at path and returns the parsed config.
// A missing file is not an error — it returns an empty (no-op) config.
func loadRotationConfig(path string, logger interface{ Printf(string, ...any) }) (*rotationConfig, error) {
	rc := &rotationConfig{angles: make(map[string]int), path: path}
	if err := rc.reload(logger); err != nil {
		return nil, err
	}
	return rc, nil
}

// reload re-reads the config file in place. Safe to call concurrently.
func (rc *rotationConfig) reload(logger interface{ Printf(string, ...any) }) error {
	data, err := os.ReadFile(rc.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // optional file — silently skip
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

// get returns the configured clockwise rotation (0/90/180/270) for a camera ID.
func (rc *rotationConfig) get(cameraID string) int {
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

// applyRotation decodes the JPEG, rotates by the configured angle, and
// re-encodes. Returns the original bytes unchanged if degrees == 0 or on error.
func applyRotation(data []byte, degrees int) []byte {
	if degrees == 0 {
		return data
	}

	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data // pass original through on decode failure
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
		return data // pass original through on encode failure
	}
	return buf.Bytes()
}

// rotate90 rotates src 90° clockwise.
// Output size: H × W (original height becomes new width).
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

// rotate180 rotates src 180°.
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

// rotate270 rotates src 270° clockwise (= 90° counter-clockwise).
// Output size: H × W.
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
