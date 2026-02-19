package gowebper_test

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/eringen/gowebper"
	"github.com/eringen/gowebper/internal/vp8ldec"
)

func TestLargeFile_test2(t *testing.T) {
	f, err := os.Open("test2.png")
	if err != nil {
		t.Skip("test2.png not found")
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	for _, level := range []int{0, 3, 6} {
		t.Run("level"+string(rune('0'+level)), func(t *testing.T) {
			data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: level})
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := vp8ldec.DecodeBytes(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			// Check a handful of pixels.
			for _, pt := range [][2]int{{0, 0}, {1, 0}, {100, 100}, {500, 500}} {
				wr, wg, wb, wa := img.At(pt[0], pt[1]).RGBA()
				gr, gg, gb, ga := decoded.At(pt[0], pt[1]).RGBA()
				if wr != gr || wg != gg || wb != gb || wa != ga {
					t.Errorf("pixel(%d,%d): want (%d,%d,%d,%d) got (%d,%d,%d,%d)",
						pt[0], pt[1], wr>>8, wg>>8, wb>>8, wa>>8, gr>>8, gg>>8, gb>>8, ga>>8)
				}
			}
		})
	}
}

func TestLevel0_largeGradient(t *testing.T) {
	for _, sz := range [][2]int{{2047, 1003}, {2048, 1024}} {
		w, h := sz[0], sz[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			img := makeGradient(w, h)
			got := roundTrip(t, img, 0)
			compareImages(t, img, got, "level0 large gradient")
		})
	}
}

func TestLevel0_test2Crop(t *testing.T) {
	f, err := os.Open("test2.png")
	if err != nil {
		t.Skip("test2.png not found")
	}
	full, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Try progressively larger sub-images until failure.
	for _, sz := range [][2]int{{64, 64}, {128, 128}, {256, 256}, {512, 512}, {1024, 512}, {2047, 1003}} {
		w, h := sz[0], sz[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			sub := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					sub.Set(x, y, full.At(x, y))
				}
			}
			got := roundTrip(t, sub, 0)
			compareImages(t, sub, got, fmt.Sprintf("level0 crop %dx%d", w, h))
		})
	}
}

func TestLevel0_skewedDistribution(t *testing.T) {
	// Reproduce the failure: a large image where one channel value dominates,
	// producing a deeply unbalanced Huffman tree that triggers limitLengths.
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			// G=68 is dominant; the diagonal has all 256 G values.
			g := uint8(68)
			if x == y {
				g = uint8(x)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: g, G: g, B: g, A: 255})
		}
	}
	got := roundTrip(t, img, 0)
	compareImages(t, img, got, "level0 skewed")
}
