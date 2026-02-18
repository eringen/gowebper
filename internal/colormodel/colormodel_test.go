package colormodel

import (
	"image"
	"image/color"
	"testing"
)

func TestToARGB_solidOpaque(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 255, B: 0, A: 255})
	img.SetNRGBA(0, 1, color.NRGBA{R: 0, G: 0, B: 255, A: 255})
	img.SetNRGBA(1, 1, color.NRGBA{R: 128, G: 64, B: 32, A: 255})

	pixels := ToARGB(img)
	if len(pixels) != 4 {
		t.Fatalf("expected 4 pixels, got %d", len(pixels))
	}
	want := []uint32{
		0xFFFF0000,
		0xFF00FF00,
		0xFF0000FF,
		0xFF804020,
	}
	for i, w := range want {
		if pixels[i] != w {
			t.Errorf("pixel %d: got 0x%08X, want 0x%08X", i, pixels[i], w)
		}
	}
}

func TestToARGB_transparent_NRGBA(t *testing.T) {
	// NRGBA stores non-premultiplied values; ToARGB should preserve them exactly.
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 100, G: 150, B: 200, A: 128})

	pixels := ToARGB(img)
	if len(pixels) != 1 {
		t.Fatalf("expected 1 pixel, got %d", len(pixels))
	}
	// a=128=0x80, r=100=0x64, g=150=0x96, b=200=0xC8
	want := uint32(0x806496C8)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestToARGB_RGBA_opaque(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	pixels := ToARGB(img)
	if len(pixels) != 1 {
		t.Fatalf("expected 1 pixel, got %d", len(pixels))
	}
	// RGBA stores premultiplied but with A=255, R/G/B are unchanged.
	want := uint32(0xFF0A141E)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestToARGB_Gray(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 1, 1))
	img.SetGray(0, 0, color.Gray{Y: 0xAB})

	pixels := ToARGB(img)
	if len(pixels) != 1 {
		t.Fatalf("expected 1 pixel, got %d", len(pixels))
	}
	// Gray: lum replicated into R, G, B; A=255.
	want := uint32(0xFFABABAB)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestHasAlpha_opaque(t *testing.T) {
	opaque := []uint32{0xFF000000, 0xFFFFFFFF}
	if HasAlpha(opaque) {
		t.Error("expected no alpha in opaque pixels")
	}
}

func TestHasAlpha_transparent(t *testing.T) {
	transparent := []uint32{0xFF000000, 0x80000000}
	if !HasAlpha(transparent) {
		t.Error("expected alpha detected in transparent pixels")
	}
}

func TestHasAlpha_fullyTransparent(t *testing.T) {
	fullyTransparent := []uint32{0x00000000}
	if !HasAlpha(fullyTransparent) {
		t.Error("expected alpha detected in fully transparent pixel")
	}
}

func TestToARGB_empty(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 0, 0))
	pixels := ToARGB(img)
	if pixels != nil {
		t.Errorf("expected nil for empty image, got %v", pixels)
	}
}

func TestToARGB_singlePixelOpaque(t *testing.T) {
	img := image.NewNRGBA(image.Rect(5, 5, 6, 6))
	img.SetNRGBA(5, 5, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	pixels := ToARGB(img)
	if len(pixels) != 1 {
		t.Fatalf("expected 1 pixel, got %d", len(pixels))
	}
	want := uint32(0xFF0A141E)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestToARGB_NRGBA64(t *testing.T) {
	img := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	// Set with 16-bit values; high bytes should be extracted.
	img.SetNRGBA64(0, 0, color.NRGBA64{R: 0xFF00, G: 0x8000, B: 0x4000, A: 0xFFFF})

	pixels := ToARGB(img)
	if len(pixels) != 1 {
		t.Fatalf("expected 1 pixel, got %d", len(pixels))
	}
	// High bytes: A=0xFF, R=0xFF, G=0x80, B=0x40.
	want := uint32(0xFFFF8040)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}
