package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestGridLayout(t *testing.T) {
	cases := []struct {
		n, cols, rows int
	}{
		{0, 0, 0},
		{1, 1, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{5, 3, 2},
		{6, 3, 2},
		{7, 3, 3},
		{9, 3, 3},
	}
	for _, c := range cases {
		cols, rows := gridLayout(c.n)
		if cols != c.cols || rows != c.rows {
			t.Errorf("gridLayout(%d) = %dx%d, want %dx%d", c.n, cols, rows, c.cols, c.rows)
		}
	}
}

func TestFitImagePreservesAspect(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("fitImage(640x480→320x240) = %dx%d, want 320x240", b.Dx(), b.Dy())
	}
}

func TestFitImageLetterboxes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 360))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 180 {
		t.Errorf("fitImage(640x360→320x240) = %dx%d, want 320x180", b.Dx(), b.Dy())
	}
}

func TestFitImageNoScaleNeeded(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 240))
	out := fitImage(src, 320, 240)
	if out != image.Image(src) {
		t.Error("fitImage returned a copy when source already fits")
	}
}

func TestComposeCanvasDimensions(t *testing.T) {
	ids := []string{"0", "1", "2", "3"}
	frames := map[string]*cameraFrame{}
	for _, id := range ids {
		frames[id] = &cameraFrame{img: image.NewRGBA(image.Rect(0, 0, 320, 240))}
	}

	cols, rows := gridLayout(len(ids))
	canvas := composeCanvas(ids, frames, cols, rows, 320, 240)
	b := canvas.Bounds()

	if b.Dx() != 640 || b.Dy() != 480 {
		t.Errorf("canvas dimensions = %dx%d, want 640x480", b.Dx(), b.Dy())
	}
}

func TestComposeCanvasPixelPlacement(t *testing.T) {
	redImg := solidColorImage(color.RGBA{255, 0, 0, 255}, 320, 240)
	blueImg := solidColorImage(color.RGBA{0, 0, 255, 255}, 320, 240)

	ids := []string{"0", "1"}
	frames := map[string]*cameraFrame{
		"0": {img: redImg},
		"1": {img: blueImg},
	}

	cols, rows := gridLayout(2)
	canvas := composeCanvas(ids, frames, cols, rows, 320, 240)

	r, g, b, _ := canvas.At(160, 120).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("left cell pixel = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}

	r, g, b, _ = canvas.At(480, 120).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 255 {
		t.Errorf("right cell pixel = (%d,%d,%d), want blue", r>>8, g>>8, b>>8)
	}
}

func TestEncodeJPEGBytesRoundtrip(t *testing.T) {
	src := solidColorImage(color.RGBA{128, 64, 32, 255}, 64, 64)

	encoded, err := encodeJPEGBytes(src)
	if err != nil {
		t.Fatalf("encodeJPEGBytes returned error: %v", err)
	}
	if !looksLikeJPEG(encoded) {
		t.Fatal("encoded bytes do not look like JPEG")
	}
}

func TestDecodeJPEG(t *testing.T) {
	src := solidColorImage(color.RGBA{200, 100, 50, 255}, 32, 32)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	out, err := decodeJPEG(buf.Bytes())
	if err != nil {
		t.Fatalf("decodeJPEG returned error: %v", err)
	}
	b := out.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("decoded image = %dx%d, want 32x32", b.Dx(), b.Dy())
	}
}

func solidColorImage(c color.RGBA, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}
