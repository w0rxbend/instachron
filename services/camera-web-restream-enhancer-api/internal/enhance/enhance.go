// Package enhance applies brightness, contrast, and sharpness corrections to JPEG frames.
package enhance

import (
	"bytes"
	"image"
	"sync"

	"github.com/disintegration/imaging"
)

// Config holds the tunable parameters for the image enhancement pipeline.
type Config struct {
	// Sharpen is the unsharp-mask sigma passed to imaging.Sharpen.
	// 0 disables sharpening. Typical range: 0.5–3.0.
	Sharpen float64

	// DarkThreshold is the average BT.601 luminance (0–1) below which adaptive
	// brightness and contrast corrections are applied. 0 disables adaptive mode.
	DarkThreshold float64

	// BrightnessMax is the maximum brightness adjustment (0–100 %) applied when
	// the image is completely black (lum == 0). Scales linearly with darkness.
	BrightnessMax float64

	// ContrastMax is the maximum contrast adjustment (0–100 %) applied under
	// the same conditions as BrightnessMax.
	ContrastMax float64

	// JPEGQuality is the re-encode quality (1–100) for output frames.
	JPEGQuality int
}

// Processor applies the enhancement pipeline to raw JPEG frames.
// It implements restream.Processor — Process is safe for concurrent use.
type Processor struct {
	cfg     Config
	bufPool sync.Pool
}

// New returns a Processor configured with cfg.
func New(cfg Config) *Processor {
	return &Processor{
		cfg:     cfg,
		bufPool: sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
}

// Process implements restream.Processor. It enhances jpeg and calls push with
// the result. On any error the original frame is passed through unchanged.
func (e *Processor) Process(jpeg []byte, push func([]byte)) {
	push(e.process(jpeg))
}

func (e *Processor) process(jpeg []byte) []byte {
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return jpeg
	}

	if e.cfg.DarkThreshold > 0 {
		lum := avgLuminance32(img)
		if lum < e.cfg.DarkThreshold {
			factor := (e.cfg.DarkThreshold - lum) / e.cfg.DarkThreshold
			if e.cfg.BrightnessMax > 0 {
				img = imaging.AdjustBrightness(img, factor*e.cfg.BrightnessMax)
			}
			if e.cfg.ContrastMax > 0 {
				img = imaging.AdjustContrast(img, factor*e.cfg.ContrastMax)
			}
		}
	}

	if e.cfg.Sharpen > 0 {
		img = imaging.Sharpen(img, e.cfg.Sharpen)
	}

	buf := e.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer e.bufPool.Put(buf)

	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(e.cfg.JPEGQuality)); err != nil {
		return jpeg
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}

// avgLuminance32 computes the average BT.601 luminance (0–1) by downsampling
// to a 32-pixel-wide thumbnail using the fast Box filter.
func avgLuminance32(img image.Image) float64 {
	thumb := imaging.Resize(img, 32, 0, imaging.Box)
	pix := thumb.Pix
	n := len(pix) / 4
	if n == 0 {
		return 0
	}
	var sum uint64
	for i := 0; i < len(pix); i += 4 {
		r, g, b := uint64(pix[i]), uint64(pix[i+1]), uint64(pix[i+2])
		sum += 299*r + 587*g + 114*b
	}
	return float64(sum) / float64(n) / 1000.0 / 255.0
}
