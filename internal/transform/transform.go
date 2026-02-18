// Package transform implements VP8L image transforms.
// The forward transforms are applied by the encoder; the inverse transforms
// are applied by the decoder (vp8ldec).
package transform

// SubtractGreenForward applies the SubtractGreen forward transform in-place.
// For each pixel: R -= G, B -= G (all uint8 modular arithmetic).
func SubtractGreenForward(pixels []uint32) {
	for i, p := range pixels {
		a := p >> 24
		r := (p >> 16) & 0xFF
		g := (p >> 8) & 0xFF
		b := p & 0xFF
		r = (r - g) & 0xFF
		b = (b - g) & 0xFF
		pixels[i] = a<<24 | r<<16 | g<<8 | b
	}
}

// SubtractGreenInverse reverses the SubtractGreen transform in-place.
// For each pixel: R += G, B += G (uint8 modular).
func SubtractGreenInverse(pixels []uint32) {
	for i, p := range pixels {
		a := p >> 24
		r := (p >> 16) & 0xFF
		g := (p >> 8) & 0xFF
		b := p & 0xFF
		r = (r + g) & 0xFF
		b = (b + g) & 0xFF
		pixels[i] = a<<24 | r<<16 | g<<8 | b
	}
}

// BuildPredictorModes chooses a predictor mode for each tile and returns the
// tile mode array without modifying the input pixel buffer.
// Each tile covers a (1<<tileBits) x (1<<tileBits) region.
func BuildPredictorModes(pixels []uint32, width, height, tileBits int) []int {
	return buildPredictorModesOnly(pixels, width, height, tileBits)
}

// buildPredictorModesOnly returns the modes without touching the input pixels.
func buildPredictorModesOnly(pixels []uint32, width, height, tileBits int) []int {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits
	tileH := (height + tileSize - 1) >> tileBits
	modes := make([]int, tileW*tileH)

	// We score each mode on a scratch copy to avoid modifying original pixels.
	// For scoring we apply the forward transform on the scratch copy and measure
	// the resulting residual entropy as a heuristic cost (sum of abs-valued bytes).
	scratch := make([]uint32, width*height)

	for ty := 0; ty < tileH; ty++ {
		for tx := 0; tx < tileW; tx++ {
			bestMode := 0
			bestCost := int64(1<<62 - 1)

			x0 := tx * tileSize
			y0 := ty * tileSize
			x1 := x0 + tileSize
			if x1 > width {
				x1 = width
			}
			y1 := y0 + tileSize
			if y1 > height {
				y1 = height
			}

			for mode := 0; mode < 14; mode++ {
				copy(scratch, pixels)
				applyModeToTile(scratch, width, x0, y0, x1, y1, mode)
				cost := tileCost(scratch, x0, y0, x1, y1, width)
				if cost < bestCost {
					bestCost = cost
					bestMode = mode
				}
			}
			modes[ty*tileW+tx] = bestMode
		}
	}
	return modes
}

// applyModeToTile applies predictor subtraction for the given mode to a tile
// region of a scratch buffer (which is a copy of the original pixels).
// Only pixels within [x0,x1) x [y0,y1) are modified.
func applyModeToTile(pixels []uint32, width, x0, y0, x1, y1, mode int) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x == 0 && y == 0 {
				continue
			}
			pred := predictorForEncoder(mode, x, y, width, pixels)
			cur := pixels[y*width+x]
			pixels[y*width+x] = subARGB(cur, pred)
		}
	}
}

// tileCost computes a heuristic entropy cost for a tile (sum of |signed byte| values).
func tileCost(pixels []uint32, x0, y0, x1, y1, width int) int64 {
	var cost int64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			p := pixels[y*width+x]
			cost += absByte(p >> 24)
			cost += absByte(p >> 16)
			cost += absByte(p >> 8)
			cost += absByte(p)
		}
	}
	return cost
}

func absByte(v uint32) int64 {
	b := int64(int8(v & 0xFF))
	if b < 0 {
		return -b
	}
	return b
}

// PredictorForward applies the predictor transform forward for encoding.
// Each pixel[i] becomes pixel[i] - prediction(original_neighbors[i]).
// The prediction uses original (pre-transform) neighbor values, so we work
// from a copy and write residuals into the output buffer.
func PredictorForward(pixels []uint32, width, height, tileBits int, modes []int) {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits

	// Keep a read-only copy of the original values for prediction.
	orig := make([]uint32, len(pixels))
	copy(orig, pixels)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if x == 0 && y == 0 {
				pixels[0] = subARGB(orig[0], 0xFF000000)
				continue
			}
			mode := getTileMode(modes, x, y, tileW, tileBits)
			pred := predictorForEncoder(mode, x, y, width, orig)
			pixels[idx] = subARGB(orig[idx], pred)
		}
	}
}

// PredictorInverse reverses the predictor transform for decoding.
// Each residual pixel[i] becomes pixel[i] + prediction(already_decoded[i]).
// Pixels are decoded in raster order so neighbors are already restored.
func PredictorInverse(pixels []uint32, width, height, tileBits int, modes []int) {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if x == 0 && y == 0 {
				pixels[0] = addARGB(pixels[0], 0xFF000000)
				continue
			}
			mode := getTileMode(modes, x, y, tileW, tileBits)
			pred := predictorForDecoder(mode, x, y, width, pixels)
			pixels[idx] = addARGB(pixels[idx], pred)
		}
	}
}

// getTileMode returns the predictor mode for pixel (x,y), applying VP8L border
// rules: top row uses "left" predictor, left column uses "top" predictor.
func getTileMode(modes []int, x, y, tileW, tileBits int) int {
	if y == 0 {
		return 1 // left predictor for top row
	}
	if x == 0 {
		return 2 // top predictor for left column
	}
	tx := x >> tileBits
	ty := y >> tileBits
	return modes[ty*tileW+tx]
}

// predictorForEncoder computes the prediction for a pixel using original
// (pre-transform) neighbor values. The orig slice is the untransformed image.
func predictorForEncoder(mode, x, y, width int, orig []uint32) uint32 {
	return computePredictor(mode, x, y, width, orig)
}

// predictorForDecoder computes the prediction for a pixel using already-decoded
// neighbor values. The pixels slice is updated in-place during decoding.
func predictorForDecoder(mode, x, y, width int, pixels []uint32) uint32 {
	return computePredictor(mode, x, y, width, pixels)
}

// computePredictor computes the prediction value for pixel (x,y) using the
// given mode and the provided pixel buffer.
func computePredictor(mode, x, y, width int, pix []uint32) uint32 {
	left := func() uint32 {
		if x > 0 {
			return pix[y*width+x-1]
		}
		return 0xFF000000
	}
	top := func() uint32 {
		if y > 0 {
			return pix[(y-1)*width+x]
		}
		return 0xFF000000
	}
	topLeft := func() uint32 {
		if y > 0 && x > 0 {
			return pix[(y-1)*width+x-1]
		}
		return 0xFF000000
	}
	topRight := func() uint32 {
		if y > 0 && x+1 < width {
			return pix[(y-1)*width+x+1]
		}
		return top()
	}

	switch mode {
	case 0:
		return 0xFF000000
	case 1:
		return left()
	case 2:
		return top()
	case 3:
		return topRight()
	case 4:
		return topLeft()
	case 5:
		return average2(average2(left(), topRight()), top())
	case 6:
		return average2(left(), topLeft())
	case 7:
		return average2(left(), top())
	case 8:
		return average2(topLeft(), top())
	case 9:
		return average2(top(), topRight())
	case 10:
		return average2(average2(left(), topLeft()), average2(top(), topRight()))
	case 11:
		return selectPredictor(left(), top(), topLeft())
	case 12:
		return clampAddSubFull(left(), top(), topLeft())
	case 13:
		return clampAddSubHalf(average2(left(), top()), topLeft())
	default:
		return top()
	}
}

func addARGB(a, b uint32) uint32 {
	return uint32(uint8(a>>24)+uint8(b>>24))<<24 |
		uint32(uint8(a>>16)+uint8(b>>16))<<16 |
		uint32(uint8(a>>8)+uint8(b>>8))<<8 |
		uint32(uint8(a)+uint8(b))
}

func subARGB(a, b uint32) uint32 {
	return uint32(uint8(a>>24)-uint8(b>>24))<<24 |
		uint32(uint8(a>>16)-uint8(b>>16))<<16 |
		uint32(uint8(a>>8)-uint8(b>>8))<<8 |
		uint32(uint8(a)-uint8(b))
}

func average2(a, b uint32) uint32 {
	return (((a ^ b) & 0xFEFEFEFE) >> 1) + (a & b)
}

func selectPredictor(left, top, topLeft uint32) uint32 {
	pLeft := abs8i(top>>24, topLeft>>24) + abs8i(top>>16, topLeft>>16) +
		abs8i(top>>8, topLeft>>8) + abs8i(top, topLeft)
	pTop := abs8i(left>>24, topLeft>>24) + abs8i(left>>16, topLeft>>16) +
		abs8i(left>>8, topLeft>>8) + abs8i(left, topLeft)
	if pLeft <= pTop {
		return left
	}
	return top
}

func abs8i(a, b uint32) int32 {
	d := int32(a&0xFF) - int32(b&0xFF)
	if d < 0 {
		return -d
	}
	return d
}

func clampAddSubFull(left, top, topLeft uint32) uint32 {
	ra := clamp8(int32(left>>24&0xFF) + int32(top>>24&0xFF) - int32(topLeft>>24&0xFF))
	rr := clamp8(int32(left>>16&0xFF) + int32(top>>16&0xFF) - int32(topLeft>>16&0xFF))
	rg := clamp8(int32(left>>8&0xFF) + int32(top>>8&0xFF) - int32(topLeft>>8&0xFF))
	rb := clamp8(int32(left&0xFF) + int32(top&0xFF) - int32(topLeft&0xFF))
	return ra<<24 | rr<<16 | rg<<8 | rb
}

func clampAddSubHalf(avg, topLeft uint32) uint32 {
	ra := clamp8(int32(avg>>24&0xFF) + (int32(avg>>24&0xFF)-int32(topLeft>>24&0xFF))/2)
	rr := clamp8(int32(avg>>16&0xFF) + (int32(avg>>16&0xFF)-int32(topLeft>>16&0xFF))/2)
	rg := clamp8(int32(avg>>8&0xFF) + (int32(avg>>8&0xFF)-int32(topLeft>>8&0xFF))/2)
	rb := clamp8(int32(avg&0xFF) + (int32(avg&0xFF)-int32(topLeft&0xFF))/2)
	return ra<<24 | rr<<16 | rg<<8 | rb
}

func clamp8(v int32) uint32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint32(v)
}

// BuildCrossColorMultipliers computes the cross-color multiplier for each tile.
// Returns an array of packed (greenToRed<<16 | greenToBlue) uint32 values.
func BuildCrossColorMultipliers(pixels []uint32, width, height, tileBits int) []uint32 {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits
	tileH := (height + tileSize - 1) >> tileBits
	multipliers := make([]uint32, tileW*tileH)

	for ty := 0; ty < tileH; ty++ {
		for tx := 0; tx < tileW; tx++ {
			x0 := tx * tileSize
			y0 := ty * tileSize
			x1 := x0 + tileSize
			if x1 > width {
				x1 = width
			}
			y1 := y0 + tileSize
			if y1 > height {
				y1 = height
			}

			g2r, g2b := estimateCrossColorMultipliers(pixels, width, x0, y0, x1, y1)
			multipliers[ty*tileW+tx] = uint32(uint8(g2r))<<16 | uint32(uint8(g2b))
		}
	}
	return multipliers
}

// estimateCrossColorMultipliers finds the best greenToRed and greenToBlue
// multipliers for a tile using a simple linear regression.
func estimateCrossColorMultipliers(pixels []uint32, width, x0, y0, x1, y1 int) (greenToRed, greenToBlue int8) {
	var sumGG, sumGR, sumGB int64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			p := pixels[y*width+x]
			g := int64(int8(p >> 8))
			r := int64(int8(p >> 16))
			b := int64(int8(p))
			sumGG += g * g
			sumGR += g * r
			sumGB += g * b
		}
	}
	if sumGG == 0 {
		return 0, 0
	}
	// Scale by 32 (VP8L uses green*multiplier>>5 for correction).
	g2r := int8(clampInt64(sumGR*32/sumGG, -128, 127))
	g2b := int8(clampInt64(sumGB*32/sumGG, -128, 127))
	return g2r, g2b
}

func clampInt64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// CrossColorForward applies the cross-color transform forward for encoding.
// For each pixel: R -= (G * greenToRed) >> 5; B -= (G * greenToBlue) >> 5.
func CrossColorForward(pixels []uint32, width, height, tileBits int, multipliers []uint32) {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tx := x >> tileBits
			ty := y >> tileBits
			m := multipliers[ty*tileW+tx]
			greenToRed := int8(m >> 16)
			greenToBlue := int8(m)

			p := pixels[y*width+x]
			g := int32(int8(p >> 8))
			r := uint8(p>>16) - uint8(int32(greenToRed)*g>>5)
			b := uint8(p) - uint8(int32(greenToBlue)*g>>5)
			pixels[y*width+x] = (p & 0xFF00FF00) | uint32(r)<<16 | uint32(b)
		}
	}
}

// CrossColorInverse reverses the cross-color transform for decoding.
func CrossColorInverse(pixels []uint32, width, height, tileBits int, multipliers []uint32) {
	tileSize := 1 << tileBits
	tileW := (width + tileSize - 1) >> tileBits

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tx := x >> tileBits
			ty := y >> tileBits
			m := multipliers[ty*tileW+tx]
			greenToRed := int8(m >> 16)
			greenToBlue := int8(m)

			p := pixels[y*width+x]
			g := int32(int8(p >> 8))
			r := uint8(p>>16) + uint8(int32(greenToRed)*g>>5)
			b := uint8(p) + uint8(int32(greenToBlue)*g>>5)
			pixels[y*width+x] = (p & 0xFF00FF00) | uint32(r)<<16 | uint32(b)
		}
	}
}
