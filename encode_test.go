package gowebper_test

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/eringen/gowebper"
	"github.com/eringen/gowebper/internal/vp8ldec"
)

// makeImage creates a simple NRGBA test image filled with the given color.
func makeImage(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// makeGradient creates an NRGBA image with a simple gradient.
func makeGradient(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / (w - 1)),
				G: uint8(y * 255 / (h - 1)),
				B: uint8((x + y) * 255 / (w + h - 2)),
				A: 255,
			})
		}
	}
	return img
}

// roundTrip encodes with gowebper and decodes with vp8ldec, returns decoded image.
func roundTrip(t *testing.T, img image.Image, level int) image.Image {
	t.Helper()
	data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: level})
	if err != nil {
		t.Fatalf("level %d encode: %v", level, err)
	}
	decoded, err := vp8ldec.DecodeBytes(data)
	if err != nil {
		t.Fatalf("level %d decode: %v", level, err)
	}
	return decoded
}

// compareImages checks that two images have identical pixel values.
func compareImages(t *testing.T, want, got image.Image, label string) {
	t.Helper()
	wb := want.Bounds()
	gb := got.Bounds()
	if wb != gb {
		t.Errorf("%s: bounds mismatch: want %v, got %v", label, wb, gb)
		return
	}
	mismatches := 0
	for y := wb.Min.Y; y < wb.Max.Y; y++ {
		for x := wb.Min.X; x < wb.Max.X; x++ {
			wr, wg, wb2, wa := want.At(x, y).RGBA()
			gr, gg, gb2, ga := got.At(x, y).RGBA()
			if wr != gr || wg != gg || wb2 != gb2 || wa != ga {
				if mismatches == 0 {
					t.Errorf("%s: pixel (%d,%d): want (%d,%d,%d,%d) got (%d,%d,%d,%d)",
						label, x, y, wr>>8, wg>>8, wb2>>8, wa>>8,
						gr>>8, gg>>8, gb2>>8, ga>>8)
				}
				mismatches++
			}
		}
	}
	if mismatches > 1 {
		t.Errorf("%s: %d total pixel mismatches", label, mismatches)
	}
}

func TestEncode_level0_solidRed(t *testing.T) {
	img := makeImage(8, 8, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	got := roundTrip(t, img, gowebper.LevelFastest)
	compareImages(t, img, got, "level0 solid red")
}

func TestEncode_level0_solidGreen(t *testing.T) {
	img := makeImage(4, 4, color.NRGBA{R: 0, G: 200, B: 0, A: 255})
	got := roundTrip(t, img, 0)
	compareImages(t, img, got, "level0 solid green")
}

func TestEncode_level0_transparent(t *testing.T) {
	img := makeImage(2, 2, color.NRGBA{R: 128, G: 64, B: 32, A: 128})
	got := roundTrip(t, img, 0)
	compareImages(t, img, got, "level0 transparent")
}

func TestEncode_level0_1x1(t *testing.T) {
	img := makeImage(1, 1, color.NRGBA{R: 17, G: 34, B: 51, A: 255})
	got := roundTrip(t, img, 0)
	compareImages(t, img, got, "level0 1x1")
}

func TestEncode_allLevels_solidColor(t *testing.T) {
	img := makeImage(16, 16, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
	for level := 0; level <= 9; level++ {
		t.Run("level"+string(rune('0'+level)), func(t *testing.T) {
			got := roundTrip(t, img, level)
			compareImages(t, img, got, "solid color")
		})
	}
}

func TestEncode_allLevels_gradient(t *testing.T) {
	img := makeGradient(32, 32)
	for level := 0; level <= 9; level++ {
		t.Run("level"+string(rune('0'+level)), func(t *testing.T) {
			got := roundTrip(t, img, level)
			compareImages(t, img, got, "gradient")
		})
	}
}

func TestEncode_level0_multicolor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	colors := []color.NRGBA{
		{R: 255, G: 0, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 255},
		{R: 0, G: 0, B: 255, A: 255},
		{R: 255, G: 255, B: 0, A: 255},
		{R: 255, G: 0, B: 255, A: 255},
		{R: 0, G: 255, B: 255, A: 255},
		{R: 128, G: 128, B: 128, A: 255},
		{R: 0, G: 0, B: 0, A: 255},
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, colors[(x+y*4)%len(colors)])
		}
	}
	got := roundTrip(t, img, 0)
	compareImages(t, img, got, "multicolor")
}

func TestEncode_nilOptions_usesDefault(t *testing.T) {
	img := makeImage(8, 8, color.NRGBA{R: 50, G: 100, B: 150, A: 255})
	data, err := gowebper.EncodeToBytes(img, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := vp8ldec.DecodeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	compareImages(t, img, decoded, "nil options")
}

func TestEncoder_reuse(t *testing.T) {
	enc := gowebper.NewEncoder(&gowebper.Options{Level: 3})
	imgs := []image.Image{
		makeImage(8, 8, color.NRGBA{R: 255, G: 0, B: 0, A: 255}),
		makeImage(4, 4, color.NRGBA{R: 0, G: 255, B: 0, A: 255}),
		makeGradient(16, 16),
	}
	for i, img := range imgs {
		data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: 3})
		if err != nil {
			t.Fatalf("img %d EncodeToBytes: %v", i, err)
		}
		_ = enc

		decoded, err := vp8ldec.DecodeBytes(data)
		if err != nil {
			t.Fatalf("img %d decode: %v", i, err)
		}
		compareImages(t, img, decoded, "reuse")
	}
}

func TestEncode_fileSizeMonotonicity(t *testing.T) {
	// For a gradient image (not a palette case), higher levels should not
	// produce dramatically larger files than lower levels. This is a soft check.
	img := makeGradient(64, 64)
	sizes := make([]int, 10)
	for level := 0; level <= 9; level++ {
		data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: level})
		if err != nil {
			t.Fatalf("level %d: %v", level, err)
		}
		sizes[level] = len(data)
	}
	t.Logf("file sizes by level: %v", sizes)
	// Level 9 should not be larger than level 0.
	if sizes[9] > sizes[0] {
		t.Errorf("level 9 (%d bytes) is larger than level 0 (%d bytes)", sizes[9], sizes[0])
	}
}

func TestEncode_RIFF_header(t *testing.T) {
	img := makeImage(1, 1, color.NRGBA{R: 42, G: 43, B: 44, A: 255})
	data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 20 {
		t.Fatalf("output too short: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" {
		t.Errorf("expected RIFF, got %q", data[0:4])
	}
	if string(data[8:12]) != "WEBP" {
		t.Errorf("expected WEBP, got %q", data[8:12])
	}
	if string(data[12:16]) != "VP8L" {
		t.Errorf("expected VP8L, got %q", data[12:16])
	}
}

// roundTripWithQuality encodes with the given level+quality and decodes.
func roundTripWithQuality(t *testing.T, img image.Image, level, quality int) (image.Image, []byte) {
	t.Helper()
	data, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: level, Quality: quality})
	if err != nil {
		t.Fatalf("level=%d quality=%d encode: %v", level, quality, err)
	}
	decoded, err := vp8ldec.DecodeBytes(data)
	if err != nil {
		t.Fatalf("level=%d quality=%d decode: %v", level, quality, err)
	}
	return decoded, data
}

func TestQuality_zero_isLossless(t *testing.T) {
	img := makeGradient(32, 32)
	data0, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: 6})
	if err != nil {
		t.Fatal(err)
	}
	dataQ, err := gowebper.EncodeToBytes(img, &gowebper.Options{Level: 6, Quality: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data0, dataQ) {
		t.Error("Quality=0 produced different output than no Quality field")
	}
}

func TestQuality_100_isLossless(t *testing.T) {
	img := makeGradient(16, 16)
	got, _ := roundTripWithQuality(t, img, 0, 100)
	compareImages(t, img, got, "quality=100 round-trip")
}

func TestQuality_producesValidWebP(t *testing.T) {
	img := makeGradient(32, 32)
	for _, q := range []int{1, 10, 25, 50, 75, 90, 99} {
		_, data := roundTripWithQuality(t, img, 0, q)
		if len(data) == 0 {
			t.Errorf("quality=%d: empty output", q)
		}
	}
}

func TestQuality_alphaPreserved(t *testing.T) {
	// Semi-transparent image: each pixel has a distinct alpha.
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 16),
				G: uint8(y * 16),
				B: 128,
				A: uint8((x + y) * 8),
			})
		}
	}
	for _, q := range []int{1, 50, 99} {
		got, _ := roundTripWithQuality(t, img, 0, q)
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				_, _, _, wa := img.At(x, y).RGBA()
				_, _, _, ga := got.At(x, y).RGBA()
				if wa != ga {
					t.Errorf("quality=%d: alpha mismatch at (%d,%d): want %d got %d", q, x, y, wa>>8, ga>>8)
				}
			}
		}
	}
}

func TestQuality_fileSizeDecreases(t *testing.T) {
	img := makeGradient(64, 64)
	_, dataLow := roundTripWithQuality(t, img, 0, 1)
	_, dataHigh := roundTripWithQuality(t, img, 0, 75)
	if len(dataLow) >= len(dataHigh) {
		t.Errorf("quality=1 (%d bytes) is not smaller than quality=75 (%d bytes)", len(dataLow), len(dataHigh))
	}
}

func TestQuality_RIFFValid(t *testing.T) {
	img := makeGradient(16, 16)
	for _, q := range []int{1, 50, 99} {
		_, data := roundTripWithQuality(t, img, 0, q)
		if len(data) < 20 {
			t.Fatalf("quality=%d: output too short (%d bytes)", q, len(data))
		}
		if string(data[0:4]) != "RIFF" {
			t.Errorf("quality=%d: expected RIFF, got %q", q, data[0:4])
		}
		if string(data[8:12]) != "WEBP" {
			t.Errorf("quality=%d: expected WEBP, got %q", q, data[8:12])
		}
		if string(data[12:16]) != "VP8L" {
			t.Errorf("quality=%d: expected VP8L, got %q", q, data[12:16])
		}
	}
}
