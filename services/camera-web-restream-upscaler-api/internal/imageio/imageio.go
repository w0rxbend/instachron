// Package imageio provides JPEG codec helpers and resolution capping for the
// upscale pipeline.
package imageio

import (
	"bytes"
	"image"

	"github.com/disintegration/imaging"
)

// CapResolution downsizes img so neither dimension exceeds maxW/maxH,
// preserving aspect ratio. Returns img unchanged when already within bounds.
func CapResolution(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxW && h <= maxH {
		return img
	}
	scaleW := float64(maxW) / float64(w)
	scaleH := float64(maxH) / float64(h)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	return imaging.Resize(img, int(float64(w)*scale), int(float64(h)*scale), imaging.Lanczos)
}

// EncodeJPEG re-encodes img as JPEG at the given quality into buf (pooled by
// the caller). Returns a fresh []byte copy safe to hold after buf is recycled.
func EncodeJPEG(img image.Image, quality int, buf *bytes.Buffer) ([]byte, error) {
	buf.Reset()
	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, err
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}
