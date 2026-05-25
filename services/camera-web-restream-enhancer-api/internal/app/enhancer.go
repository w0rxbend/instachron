package app

import (
	"bytes"
	"image"
	"sync"

	"github.com/disintegration/imaging"
)

// enhancerConfig holds the tunable parameters for the image enhancement pipeline.
// All values are loaded once at startup from environment variables.
type enhancerConfig struct {
	// sharpen is the unsharp-mask sigma passed to imaging.Sharpen.
	// 0 disables sharpening. Typical range: 0.5–3.0.
	sharpen float64

	// darkThreshold is the average BT.601 luminance (0–1) below which adaptive
	// brightness and contrast corrections are applied. 0 disables adaptive mode.
	darkThreshold float64

	// brightnessMax is the maximum brightness adjustment (0–100 %) applied when
	// the image is completely black (lum == 0). Scales linearly with darkness.
	brightnessMax float64

	// contrastMax is the maximum contrast adjustment (0–100 %) applied under
	// the same conditions as brightnessMax.
	contrastMax float64

	// jpegQuality is the re-encode quality (1–100) for output frames.
	jpegQuality int
}

// enhancer applies the image enhancement pipeline to raw JPEG frames.
// It is safe for concurrent use by multiple upstream reader goroutines.
type enhancer struct {
	cfg     enhancerConfig
	bufPool sync.Pool
}

func newEnhancer(cfg enhancerConfig) *enhancer {
	return &enhancer{
		cfg:     cfg,
		bufPool: sync.Pool{New: func() any { return new(bytes.Buffer) }},
	}
}

// process decodes jpeg, runs the enhancement pipeline, and returns a new JPEG.
// On any error it returns the original bytes unchanged so the stream is never
// interrupted by a processing failure.
func (e *enhancer) process(jpeg []byte) []byte {
	img, err := imaging.Decode(bytes.NewReader(jpeg))
	if err != nil {
		return jpeg
	}

	if e.cfg.darkThreshold > 0 {
		lum := avgLuminance32(img)
		if lum < e.cfg.darkThreshold {
			// factor approaches 1 as the image approaches pure black.
			factor := (e.cfg.darkThreshold - lum) / e.cfg.darkThreshold
			if e.cfg.brightnessMax > 0 {
				img = imaging.AdjustBrightness(img, factor*e.cfg.brightnessMax)
			}
			if e.cfg.contrastMax > 0 {
				img = imaging.AdjustContrast(img, factor*e.cfg.contrastMax)
			}
		}
	}

	if e.cfg.sharpen > 0 {
		img = imaging.Sharpen(img, e.cfg.sharpen)
	}

	buf := e.bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer e.bufPool.Put(buf)

	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(e.cfg.jpegQuality)); err != nil {
		return jpeg
	}

	// Copy out of the pooled buffer before returning it.
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out
}

// avgLuminance32 computes the average BT.601 luminance (0–1) of img by first
// downsampling to a 32-pixel-wide thumbnail using the fast Box filter.
// This makes the per-frame analysis cost independent of the source resolution.
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
		// BT.601 integer approximation: (299R + 587G + 114B) / 1000
		sum += 299*r + 587*g + 114*b
	}
	return float64(sum) / float64(n) / 1000.0 / 255.0
}
