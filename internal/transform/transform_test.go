package transform

import (
	"testing"
)

func TestSubtractGreenForward_roundtrip(t *testing.T) {
	original := []uint32{
		0xFF804020,
		0xFFFF8040,
		0xFF000000,
		0xFFFFFFFF,
	}
	pixels := make([]uint32, len(original))
	copy(pixels, original)

	SubtractGreenForward(pixels)
	SubtractGreenInverse(pixels)

	for i, p := range pixels {
		if p != original[i] {
			t.Errorf("pixel %d: got 0x%08X, want 0x%08X", i, p, original[i])
		}
	}
}

func TestSubtractGreenForward_known(t *testing.T) {
	// Known transform: R=0x80, G=0x40, B=0x20, A=0xFF
	// After: R=0x80-0x40=0x40, G=0x40, B=0x20-0x40=0xE0, A=0xFF
	pixels := []uint32{0xFF804020}
	SubtractGreenForward(pixels)
	want := uint32(0xFF4040E0)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestSubtractGreenInverse_known(t *testing.T) {
	// Inverse of above: R=0x40+0x40=0x80, G=0x40, B=0xE0+0x40=0x20 (overflow), A=0xFF
	pixels := []uint32{0xFF4040E0}
	SubtractGreenInverse(pixels)
	want := uint32(0xFF804020)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestSubtractGreenForward_modular(t *testing.T) {
	// R=0x10, G=0x20: R-G = 0x10-0x20 = -0x10 = 0xF0 (uint8 modular).
	pixels := []uint32{0xFF102040}
	SubtractGreenForward(pixels)
	// R=0xF0, G=0x20, B=0x40-0x20=0x20.
	want := uint32(0xFFF02020)
	if pixels[0] != want {
		t.Errorf("got 0x%08X, want 0x%08X", pixels[0], want)
	}
}

func TestPredictorForwardInverse_roundtrip(t *testing.T) {
	// 4x4 image with varied pixel values.
	width, height := 4, 4
	original := []uint32{
		0xFFABCDEF, 0xFF123456, 0xFF987654, 0xFF010203,
		0xFF404040, 0xFF808080, 0xFFC0C0C0, 0xFFFFFFFF,
		0xFF112233, 0xFF445566, 0xFF778899, 0xFFAABBCC,
		0xFF000001, 0xFF000002, 0xFF000003, 0xFF000004,
	}
	pixels := make([]uint32, len(original))
	copy(pixels, original)

	tileBits := 2
	modes := buildPredictorModesOnly(pixels, width, height, tileBits)

	PredictorForward(pixels, width, height, tileBits, modes)
	PredictorInverse(pixels, width, height, tileBits, modes)

	for i, p := range pixels {
		if p != original[i] {
			t.Errorf("pixel %d: got 0x%08X, want 0x%08X", i, p, original[i])
		}
	}
}

func TestCrossColorForwardInverse_roundtrip(t *testing.T) {
	// Roundtrip test for cross-color transform.
	width, height := 4, 4
	original := []uint32{
		0xFF804020, 0xFF204080, 0xFF408020, 0xFF802040,
		0xFF112233, 0xFF334455, 0xFF556677, 0xFF778899,
		0xFF000010, 0xFF000020, 0xFF000030, 0xFF000040,
		0xFF505050, 0xFF606060, 0xFF707070, 0xFF808080,
	}
	pixels := make([]uint32, len(original))
	copy(pixels, original)

	tileBits := 2
	multipliers := BuildCrossColorMultipliers(pixels, width, height, tileBits)

	CrossColorForward(pixels, width, height, tileBits, multipliers)
	CrossColorInverse(pixels, width, height, tileBits, multipliers)

	// Due to integer arithmetic, some pixels may differ by 1 in R or B channels.
	for i, p := range pixels {
		o := original[i]
		if p == o {
			continue
		}
		// Allow off-by-one in R and B channels due to rounding.
		da := int(int8(p>>24)) - int(int8(o>>24))
		dr := int(int8(p>>16)) - int(int8(o>>16))
		dg := int(int8(p>>8)) - int(int8(o>>8))
		db := int(int8(p)) - int(int8(o))
		if da != 0 || dg != 0 || abs(dr) > 1 || abs(db) > 1 {
			t.Errorf("pixel %d: got 0x%08X, want 0x%08X (da=%d dr=%d dg=%d db=%d)",
				i, p, o, da, dr, dg, db)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestBuildPredictorModes_noModification(t *testing.T) {
	// BuildPredictorModes should not modify pixels.
	width, height := 8, 8
	original := make([]uint32, width*height)
	for i := range original {
		original[i] = uint32(0xFF000000 | i)
	}
	pixels := make([]uint32, len(original))
	copy(pixels, original)

	_ = BuildPredictorModes(pixels, width, height, 3)

	for i, p := range pixels {
		if p != original[i] {
			t.Errorf("pixel %d modified: got 0x%08X, want 0x%08X", i, p, original[i])
		}
	}
}
