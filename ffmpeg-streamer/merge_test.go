package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
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

func TestDiscoverCameras(t *testing.T) {
	dir := t.TempDir()

	// camera "0" with current-image.jpeg
	if err := os.MkdirAll(filepath.Join(dir, "0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "0", "current-image.jpeg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// camera "1" with current-image.jpeg
	if err := os.MkdirAll(filepath.Join(dir, "1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1", "current-image.jpeg"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// a subdir without current-image.jpeg — should be ignored
	if err := os.MkdirAll(filepath.Join(dir, "other"), 0o755); err != nil {
		t.Fatal(err)
	}

	// a file at top level — should be ignored
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err := discoverCameras(dir)
	if err != nil {
		t.Fatalf("discoverCameras returned error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "0" || ids[1] != "1" {
		t.Fatalf("discoverCameras = %v, want [0 1]", ids)
	}
}

func TestDiscoverCamerasEmptyDir(t *testing.T) {
	ids, err := discoverCameras(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty slice, got %v", ids)
	}
}

func TestDiscoverCamerasMissingDir(t *testing.T) {
	ids, err := discoverCameras("/tmp/does-not-exist-xyzzy")
	if err != nil {
		t.Fatalf("unexpected error for missing dir: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty slice, got %v", ids)
	}
}

func TestFitImagePreservesAspect(t *testing.T) {
	// 640x480 source into 320x240 cell: fits exactly, scale 0.5
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("fitImage(640x480→320x240) = %dx%d, want 320x240", b.Dx(), b.Dy())
	}
}

func TestFitImageLetterboxes(t *testing.T) {
	// 640x360 (16:9) into 320x240 (4:3): width fits at 320, height = 180 < 240
	src := image.NewRGBA(image.Rect(0, 0, 640, 360))
	out := fitImage(src, 320, 240)
	b := out.Bounds()
	if b.Dx() != 320 || b.Dy() != 180 {
		t.Errorf("fitImage(640x360→320x240) = %dx%d, want 320x180", b.Dx(), b.Dy())
	}
}

func TestFitImageNoScaleNeeded(t *testing.T) {
	// already fits exactly — should return the original image object
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

	cols, rows := gridLayout(len(ids)) // 2x2
	canvas := composeCanvas(ids, frames, cols, rows, 320, 240)
	b := canvas.Bounds()

	if b.Dx() != 640 || b.Dy() != 480 {
		t.Errorf("canvas dimensions = %dx%d, want 640x480", b.Dx(), b.Dy())
	}
}

func TestComposeCanvasPixelPlacement(t *testing.T) {
	// Camera "0" is solid red, camera "1" is solid blue.
	// In a 2×1 grid (2 cameras), "0" is on the left, "1" on the right.
	redImg := solidColorImage(color.RGBA{255, 0, 0, 255}, 320, 240)
	blueImg := solidColorImage(color.RGBA{0, 0, 255, 255}, 320, 240)

	ids := []string{"0", "1"}
	frames := map[string]*cameraFrame{
		"0": {img: redImg},
		"1": {img: blueImg},
	}

	cols, rows := gridLayout(2) // 2x1
	canvas := composeCanvas(ids, frames, cols, rows, 320, 240)

	// Left half should be red
	r, g, b, _ := canvas.At(160, 120).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("left cell pixel = (%d,%d,%d), want red", r>>8, g>>8, b>>8)
	}

	// Right half should be blue
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

func TestReadJPEGFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frame.jpeg")
	img := solidColorImage(color.RGBA{200, 100, 50, 255}, 32, 32)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	out, err := readJPEGFile(path)
	if err != nil {
		t.Fatalf("readJPEGFile returned error: %v", err)
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
