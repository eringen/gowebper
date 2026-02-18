package gowebper

// qualityToShift maps a quality value [1,100] to a bit-shift [0,7].
// quality=100 -> shift=0 (no rounding), quality=1 -> shift=7 (maximum rounding).
// Values outside [1,100] are clamped: 0 or below -> 0, 101 or above -> 0.
func qualityToShift(quality int) int {
	if quality <= 0 || quality > 100 {
		return 0
	}
	// Round-nearest integer division: s = ((100 - quality) * 7 + 49) / 99
	return ((100-quality)*7 + 49) / 99
}

// quantizePixels applies near-lossless pre-quantization in-place by rounding
// RGB channels to fewer significant bits. Alpha is never modified.
// shift=0 is a no-op. shift is clamped to [0,7].
func quantizePixels(pixels []uint32, shift int) {
	if shift <= 0 {
		return
	}
	if shift > 7 {
		shift = 7
	}
	mask := uint32((1 << shift) - 1)
	bias := mask >> 1
	invM := ^mask & 0xFF

	for i, p := range pixels {
		a := p >> 24
		r := (p >> 16) & 0xFF
		g := (p >> 8) & 0xFF
		b := p & 0xFF

		r = (r&invM | bias)
		g = (g&invM | bias)
		b = (b&invM | bias)

		pixels[i] = a<<24 | r<<16 | g<<8 | b
	}
}
