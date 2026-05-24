package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"log"
	"math"
	"sort"
	"time"
)

type cameraFrame struct {
	img image.Image
}

// feedMergedFrames composes a grid canvas from all active cameras and feeds it
// to ffmpeg at the configured frame rate. Recomposition is skipped when no new
// frames have arrived since the last canvas was built.
func feedMergedFrames(ctx context.Context, cfg config, ipc *ipcReader, writer io.Writer, logger *log.Logger) error {
	frameInterval := time.Second / time.Duration(cfg.frameRate)
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	var current []byte
	var lastVersion uint64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if v := ipc.currentVersion(); v != lastVersion {
				encoded, err := composeFromIPC(cfg, ipc, logger)
				if err != nil {
					logger.Printf("compose failed: %v", err)
				} else if encoded != nil {
					current = encoded
					lastVersion = v
				}
			}
			if len(current) == 0 {
				continue
			}
			if _, err := writer.Write(current); err != nil {
				return fmt.Errorf("write merged canvas to ffmpeg: %w", err)
			}
		}
	}
}

func composeFromIPC(cfg config, ipc *ipcReader, logger *log.Logger) ([]byte, error) {
	all := ipc.allLatest()
	if len(all) == 0 {
		return nil, nil
	}

	ids := make([]uint32, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	frames := make(map[string]*cameraFrame, len(ids))
	strIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		img, err := decodeJPEG(all[id])
		if err != nil {
			logger.Printf("decode JPEG camera=%d: %v", id, err)
			continue
		}
		key := fmt.Sprintf("%d", id)
		frames[key] = &cameraFrame{img: img}
		strIDs = append(strIDs, key)
	}

	if len(strIDs) == 0 {
		return nil, nil
	}

	cols, rows := gridLayout(len(strIDs))
	canvas := composeCanvas(strIDs, frames, cols, rows, cfg.cellWidth, cfg.cellHeight)
	encoded, err := encodeJPEGBytes(canvas)
	if err != nil {
		return nil, fmt.Errorf("encode canvas: %w", err)
	}

	logger.Printf("merged canvas: cameras=%d grid=%dx%d canvas=%dx%d bytes=%d",
		len(strIDs), cols, rows, cols*cfg.cellWidth, rows*cfg.cellHeight, len(encoded))
	return encoded, nil
}

// gridLayout returns the number of columns and rows for a grid of n items.
func gridLayout(n int) (cols, rows int) {
	if n == 0 {
		return 0, 0
	}
	cols = int(math.Ceil(math.Sqrt(float64(n))))
	rows = (n + cols - 1) / cols
	return cols, rows
}

// composeCanvas draws all camera frames in a cols×rows grid onto a single canvas.
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

func decodeJPEG(data []byte) (image.Image, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	return img, nil
}

func encodeJPEGBytes(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
