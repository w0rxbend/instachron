package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type cameraFrame struct {
	img     image.Image
	modTime time.Time
}

func feedMergedFrames(ctx context.Context, cfg config, writer io.Writer, logger *log.Logger) error {
	frameInterval := time.Second / time.Duration(cfg.frameRate)
	frameTicker := time.NewTicker(frameInterval)
	defer frameTicker.Stop()

	pollTicker := time.NewTicker(cfg.pollInterval)
	defer pollTicker.Stop()

	frames := map[string]*cameraFrame{}
	var current []byte

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pollTicker.C:
			recomposed, encoded, err := pollAndCompose(cfg, frames, logger)
			if err != nil {
				logger.Printf("compose failed: %v", err)
				continue
			}
			if recomposed {
				current = encoded
			}
		case <-frameTicker.C:
			if len(current) == 0 {
				continue
			}
			if _, err := writer.Write(current); err != nil {
				return fmt.Errorf("write merged canvas to ffmpeg: %w", err)
			}
		}
	}
}

func pollAndCompose(cfg config, frames map[string]*cameraFrame, logger *log.Logger) (bool, []byte, error) {
	cameraIDs, err := discoverCameras(cfg.frameDir)
	if err != nil {
		return false, nil, fmt.Errorf("discover cameras: %w", err)
	}

	changed := false
	for _, id := range cameraIDs {
		imgPath := filepath.Join(cfg.frameDir, id, "current-image.jpeg")
		info, err := os.Stat(imgPath)
		if err != nil {
			continue
		}

		state := frames[id]
		if state != nil && !info.ModTime().After(state.modTime) {
			continue
		}

		img, err := readJPEGFile(imgPath)
		if err != nil {
			logger.Printf("read frame camera=%s: %v", id, err)
			continue
		}

		frames[id] = &cameraFrame{img: img, modTime: info.ModTime()}
		changed = true
	}

	if !changed {
		return false, nil, nil
	}

	activeIDs := sortedCameraIDs(frames)
	if len(activeIDs) == 0 {
		return false, nil, nil
	}

	cols, rows := gridLayout(len(activeIDs))
	canvas := composeCanvas(activeIDs, frames, cols, rows, cfg.cellWidth, cfg.cellHeight)
	encoded, err := encodeJPEGBytes(canvas)
	if err != nil {
		return false, nil, fmt.Errorf("encode canvas: %w", err)
	}

	logger.Printf("merged canvas: cameras=%d grid=%dx%d canvas=%dx%d bytes=%d",
		len(activeIDs), cols, rows, cols*cfg.cellWidth, rows*cfg.cellHeight, len(encoded))
	return true, encoded, nil
}

// discoverCameras returns sorted IDs of all subdirectories in frameDir
// that contain a current-image.jpeg file.
func discoverCameras(frameDir string) ([]string, error) {
	entries, err := os.ReadDir(frameDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read frame dir: %w", err)
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(frameDir, e.Name(), "current-image.jpeg")); err == nil {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func readJPEGFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	return img, nil
}

// gridLayout returns the number of columns and rows for a grid of n items.
// It uses a roughly square layout: cols = ceil(sqrt(n)).
func gridLayout(n int) (cols, rows int) {
	if n == 0 {
		return 0, 0
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = (n + cols - 1) / cols
	return cols, rows
}

// composeCanvas draws all camera frames in a cols×rows grid onto a single canvas.
// Each camera occupies a cellW×cellH cell; frames are scaled to fit (letterboxed).
func composeCanvas(cameraIDs []string, frames map[string]*cameraFrame, cols, rows, cellW, cellH int) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH))

	for i, id := range cameraIDs {
		state, ok := frames[id]
		if !ok || state.img == nil {
			continue
		}

		scaled := fitImage(state.img, cellW, cellH)
		sb := scaled.Bounds()

		col := i % cols
		row := i / cols
		offsetX := col*cellW + (cellW-sb.Dx())/2
		offsetY := row*cellH + (cellH-sb.Dy())/2
		dstRect := image.Rect(offsetX, offsetY, offsetX+sb.Dx(), offsetY+sb.Dy())

		draw.Draw(canvas, dstRect, scaled, sb.Min, draw.Src)
	}

	return canvas
}

// fitImage scales src to fit within maxW×maxH, preserving aspect ratio (letterbox).
// Returns src unchanged if no scaling is needed.
func fitImage(src image.Image, maxW, maxH int) image.Image {
	sb := src.Bounds()
	srcW, srcH := sb.Dx(), sb.Dy()
	if srcW == 0 || srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, maxW, maxH))
	}

	scaleX := float64(maxW) / float64(srcW)
	scaleY := float64(maxH) / float64(srcH)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW == srcW && dstH == srcH {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx := sb.Min.X + x*srcW/dstW
			sy := sb.Min.Y + y*srcH/dstH
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func encodeJPEGBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortedCameraIDs(frames map[string]*cameraFrame) []string {
	ids := make([]string, 0, len(frames))
	for id := range frames {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
