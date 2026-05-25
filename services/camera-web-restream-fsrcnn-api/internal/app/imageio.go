package app

import (
	"bytes"
	"image"

	"github.com/disintegration/imaging"
)

// splitYCbCr converts img to NRGBA, extracts the Y channel as float32 [0,1]
// for FSRCNN input, and encodes Cb/Cr as grayscale NRGBA images for upscaling.
// Returns (yData, cbImg, crImg, width, height).
func splitYCbCr(img image.Image) ([]float32, *image.NRGBA, *image.NRGBA, int, int) {
	src := imaging.Clone(img) // normalize to *image.NRGBA
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	yData := make([]float32, w*h)
	cbImg := image.NewNRGBA(image.Rect(0, 0, w, h))
	crImg := image.NewNRGBA(image.Rect(0, 0, w, h))

	for row := 0; row < h; row++ {
		srcRow := row * src.Stride
		cbRow := row * cbImg.Stride
		crRow := row * crImg.Stride
		for col := 0; col < w; col++ {
			s := srcRow + col*4
			r := float64(src.Pix[s])
			g := float64(src.Pix[s+1])
			b := float64(src.Pix[s+2])

			// BT.601 full-range YCbCr
			Y := 0.299*r + 0.587*g + 0.114*b
			Cb := -0.168736*r - 0.331264*g + 0.5*b + 128.0
			Cr := 0.5*r - 0.418688*g - 0.081312*b + 128.0

			yData[row*w+col] = float32(Y / 255.0)

			cb := clamp8f(Cb)
			cr := clamp8f(Cr)

			c := cbRow + col*4
			cbImg.Pix[c], cbImg.Pix[c+1], cbImg.Pix[c+2], cbImg.Pix[c+3] = cb, cb, cb, 255
			d := crRow + col*4
			crImg.Pix[d], crImg.Pix[d+1], crImg.Pix[d+2], crImg.Pix[d+3] = cr, cr, cr, 255
		}
	}
	return yData, cbImg, crImg, w, h
}

// mergeYCbCr combines the FSRCNN-upscaled Y channel (float32 [0,1]) with
// CatmullRom-upscaled Cb/Cr grayscale images into an *image.NRGBA.
func mergeYCbCr(yUp []float32, cbUp, crUp *image.NRGBA, outW, outH int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	for row := 0; row < outH; row++ {
		cbRow := row * cbUp.Stride
		crRow := row * crUp.Stride
		dstRow := row * out.Stride
		for col := 0; col < outW; col++ {
			Y := float64(yUp[row*outW+col]) * 255.0
			Cb := float64(cbUp.Pix[cbRow+col*4])
			Cr := float64(crUp.Pix[crRow+col*4])

			// BT.601 YCbCr → RGB
			R := Y + 1.402*(Cr-128.0)
			G := Y - 0.344136*(Cb-128.0) - 0.714136*(Cr-128.0)
			B := Y + 1.772*(Cb-128.0)

			d := dstRow + col*4
			out.Pix[d] = clamp8f(R)
			out.Pix[d+1] = clamp8f(G)
			out.Pix[d+2] = clamp8f(B)
			out.Pix[d+3] = 255
		}
	}
	return out
}

// clampY clamps FSRCNN float32 output to [0, 1] in-place.
func clampY(data []float32) {
	for i, v := range data {
		if v < 0 {
			data[i] = 0
		} else if v > 1 {
			data[i] = 1
		}
	}
}

// capResolution downsizes img so neither dimension exceeds maxW/maxH,
// preserving aspect ratio. Returns img unchanged when already within bounds.
func capResolution(img image.Image, maxW, maxH int) image.Image {
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

// encodeJPEG re-encodes img as JPEG at the given quality using the provided
// buffer (pooled by the caller). Returns a fresh []byte copy.
func encodeJPEG(img image.Image, quality int, buf *bytes.Buffer) ([]byte, error) {
	buf.Reset()
	if err := imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(quality)); err != nil {
		return nil, err
	}
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

func clamp8f(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
