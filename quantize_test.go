package gowebper

import "testing"

func TestQualityToShift_boundaries(t *testing.T) {
	cases := []struct {
		quality int
		want    int
	}{
		{0, 0},
		{1, 7},
		{50, 4},
		{100, 0},
		{101, 0},
		{-1, 0},
	}
	for _, tc := range cases {
		got := qualityToShift(tc.quality)
		if got != tc.want {
			t.Errorf("qualityToShift(%d) = %d, want %d", tc.quality, got, tc.want)
		}
	}
}

func TestQuantizePixels_shiftZero_isNoop(t *testing.T) {
	pixels := []uint32{0xFF102030, 0x80AABBCC, 0x00FFFFFF}
	original := make([]uint32, len(pixels))
	copy(original, pixels)

	quantizePixels(pixels, 0)

	for i, p := range pixels {
		if p != original[i] {
			t.Errorf("pixel[%d]: shift=0 changed value from %08X to %08X", i, original[i], p)
		}
	}
}

func TestQuantizePixels_alphaUnchanged(t *testing.T) {
	alphas := []uint32{0x00, 0x40, 0x80, 0xC0, 0xFF}
	for _, a := range alphas {
		p := []uint32{a<<24 | 0x123456}
		quantizePixels(p, 4)
		got := p[0] >> 24
		if got != a {
			t.Errorf("alpha %02X changed to %02X after quantization", a, got)
		}
	}
}

func TestQuantizePixels_reducesDistinctColors(t *testing.T) {
	// Build a pixel slice with 256 distinct R values, fixed G/B/A.
	pixels := make([]uint32, 256)
	for i := range pixels {
		pixels[i] = 0xFF000000 | uint32(i)<<16
	}

	countDistinct := func(ps []uint32) int {
		seen := make(map[uint32]bool)
		for _, p := range ps {
			seen[p] = true
		}
		return len(seen)
	}

	before := countDistinct(pixels)

	pCopy := make([]uint32, len(pixels))
	copy(pCopy, pixels)
	quantizePixels(pCopy, 4) // shift=4 -> 16 distinct values

	after := countDistinct(pCopy)
	if after >= before {
		t.Errorf("distinct colors not reduced: before=%d, after=%d", before, after)
	}
}
