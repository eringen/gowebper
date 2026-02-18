// Package vp8ldec implements a minimal VP8L lossless WebP decoder.
// It is used only as a test oracle to verify the encoder output.
package vp8ldec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
)

// Decode reads a WebP file from r and returns the decoded image.
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decode(data)
}

// DecodeBytes decodes a WebP byte slice and returns the decoded image.
func DecodeBytes(data []byte) (image.Image, error) {
	return decode(data)
}

func decode(data []byte) (image.Image, error) {
	// Parse RIFF container.
	if len(data) < 20 {
		return nil, errors.New("vp8ldec: file too short")
	}
	if string(data[0:4]) != "RIFF" {
		return nil, fmt.Errorf("vp8ldec: expected RIFF, got %q", data[0:4])
	}
	if string(data[8:12]) != "WEBP" {
		return nil, fmt.Errorf("vp8ldec: expected WEBP, got %q", data[8:12])
	}
	if string(data[12:16]) != "VP8L" {
		return nil, fmt.Errorf("vp8ldec: expected VP8L chunk, got %q", data[12:16])
	}
	chunkLen := binary.LittleEndian.Uint32(data[16:20])
	if int(chunkLen)+20 > len(data) {
		return nil, errors.New("vp8ldec: VP8L chunk truncated")
	}
	return decodeVP8L(data[20 : 20+chunkLen])
}

// bitReader reads bits LSB-first from a byte slice.
type bitReader struct {
	data  []byte
	pos   int    // byte position
	bits  uint64 // bit buffer
	nBits int    // valid bits in buffer
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data}
}

func (br *bitReader) fill() {
	for br.nBits <= 56 && br.pos < len(br.data) {
		br.bits |= uint64(br.data[br.pos]) << br.nBits
		br.nBits += 8
		br.pos++
	}
}

func (br *bitReader) readBits(n int) uint32 {
	br.fill()
	v := uint32(br.bits & ((1 << n) - 1))
	br.bits >>= n
	br.nBits -= n
	return v
}

func (br *bitReader) readBit() int {
	return int(br.readBits(1))
}

// huffTree is a canonical Huffman decoder tree.
type huffTree struct {
	// For small alphabets: direct lookup by code.
	// We use a symbol table indexed by reversed code.
	maxLen  int
	table   []int16 // indexed by bit-reversed code; -1 = invalid
	lengths []int
}

func buildHuffTree(lengths []int, alphabetSize int) (*huffTree, error) {
	if alphabetSize == 0 {
		return nil, errors.New("empty alphabet")
	}
	t := &huffTree{lengths: make([]int, alphabetSize)}
	copy(t.lengths, lengths)

	// Find max length.
	for _, l := range lengths {
		if l > t.maxLen {
			t.maxLen = l
		}
	}
	if t.maxLen == 0 {
		// All zeros: use length 1 for symbol 0.
		t.maxLen = 1
		if alphabetSize > 0 {
			t.lengths[0] = 1
		}
	}

	// Count codes per length.
	blCount := make([]int, t.maxLen+1)
	for _, l := range lengths[:alphabetSize] {
		if l > 0 {
			blCount[l]++
		}
	}

	// Compute first code for each length.
	nextCode := make([]uint32, t.maxLen+2)
	code := uint32(0)
	for bits := 1; bits <= t.maxLen; bits++ {
		code = (code + uint32(blCount[bits-1])) << 1
		nextCode[bits] = code
	}

	// Build lookup table (size 2^maxLen).
	tableSize := 1 << t.maxLen
	t.table = make([]int16, tableSize)
	for i := range t.table {
		t.table[i] = -1
	}

	for sym := 0; sym < alphabetSize; sym++ {
		l := lengths[sym]
		if l == 0 {
			continue
		}
		raw := nextCode[l]
		nextCode[l]++
		// Reverse bits to get the LSB-first representation.
		rev := reverseBits(raw, l)
		// Fill all entries that have this code as a prefix.
		step := 1 << l
		for i := int(rev); i < tableSize; i += step {
			t.table[i] = int16(sym)
		}
	}
	return t, nil
}

func (t *huffTree) readSymbol(br *bitReader) (int, error) {
	br.fill()
	if t.maxLen == 0 {
		return 0, nil
	}
	peek := uint32(br.bits & ((1 << t.maxLen) - 1))
	sym := t.table[peek]
	if sym < 0 {
		return 0, fmt.Errorf("vp8ldec: invalid Huffman code 0x%x", peek)
	}
	symLen := t.lengths[sym]
	if symLen == 0 {
		return 0, errors.New("vp8ldec: symbol with zero length")
	}
	br.bits >>= symLen
	br.nBits -= symLen
	return int(sym), nil
}

func reverseBits(v uint32, n int) uint32 {
	var r uint32
	for i := 0; i < n; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}

// readHuffTree reads a Huffman tree from the bitstream.
func readHuffTree(br *bitReader, alphabetSize int) (*huffTree, error) {
	simple := br.readBit()
	if simple == 1 {
		return readSimpleHuffTree(br, alphabetSize)
	}
	return readNormalHuffTree(br, alphabetSize)
}

func readSimpleHuffTree(br *bitReader, alphabetSize int) (*huffTree, error) {
	numSymbols := int(br.readBits(1)) + 1
	symBits := symBitsForAlphabet(alphabetSize)
	sym1 := int(br.readBits(symBits))
	lengths := make([]int, alphabetSize)
	if numSymbols == 1 {
		lengths[sym1] = 1
	} else {
		sym2 := int(br.readBits(symBits))
		if sym1 > sym2 {
			sym1, sym2 = sym2, sym1
		}
		lengths[sym1] = 1
		lengths[sym2] = 1
	}
	return buildHuffTree(lengths, alphabetSize)
}

func symBitsForAlphabet(alphabetSize int) int {
	if alphabetSize <= 2 {
		return 1
	}
	return 8
}

var codeLenOrder = [19]int{17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

func readNormalHuffTree(br *bitReader, alphabetSize int) (*huffTree, error) {
	numCLCodes := int(br.readBits(4)) + 4

	// Read code-length-of-code-length values.
	clLengths := make([]int, 19)
	for i := 0; i < numCLCodes; i++ {
		clLengths[codeLenOrder[i]] = int(br.readBits(3))
	}

	// Build the code-length tree.
	clTree, err := buildHuffTree(clLengths, 19)
	if err != nil {
		return nil, fmt.Errorf("vp8ldec: code-length tree: %w", err)
	}

	// Decode the code-length sequence.
	lengths := make([]int, alphabetSize)
	i := 0
	prevLen := 0
	for i < alphabetSize {
		sym, err := clTree.readSymbol(br)
		if err != nil {
			return nil, fmt.Errorf("vp8ldec: code-length sequence at symbol %d: %w", i, err)
		}
		switch {
		case sym < 16:
			lengths[i] = sym
			prevLen = sym
			i++
		case sym == 16:
			// Repeat previous non-zero length 3..6 times.
			extra := int(br.readBits(2)) + 3
			for j := 0; j < extra && i < alphabetSize; j++ {
				lengths[i] = prevLen
				i++
			}
		case sym == 17:
			// Repeat zero 3..10 times.
			extra := int(br.readBits(3)) + 3
			for j := 0; j < extra && i < alphabetSize; j++ {
				lengths[i] = 0
				i++
			}
		case sym == 18:
			// Repeat zero 11..138 times.
			extra := int(br.readBits(7)) + 11
			for j := 0; j < extra && i < alphabetSize; j++ {
				lengths[i] = 0
				i++
			}
		}
	}
	return buildHuffTree(lengths, alphabetSize)
}

// VP8L spatial distance table (code -> (dx, dy)).
// The first 120 codes have explicit offsets; beyond 120 the offset is code-120+1 pixels back.
var distTable = [120][2]int{
	{0, 1}, {1, 0}, {1, 1}, {-1, 1}, {0, 2}, {2, 0}, {1, 2}, {-1, 2},
	{2, 1}, {-2, 1}, {2, 2}, {-2, 2}, {0, 3}, {3, 0}, {1, 3}, {-1, 3},
	{3, 1}, {-3, 1}, {2, 3}, {-2, 3}, {3, 2}, {-3, 2}, {0, 4}, {4, 0},
	{1, 4}, {-1, 4}, {4, 1}, {-4, 1}, {3, 3}, {-3, 3}, {2, 4}, {-2, 4},
	{4, 2}, {-4, 2}, {0, 5}, {3, 4}, {-3, 4}, {4, 3}, {-4, 3}, {5, 0},
	{1, 5}, {-1, 5}, {5, 1}, {-5, 1}, {2, 5}, {-2, 5}, {5, 2}, {-5, 2},
	{4, 4}, {-4, 4}, {3, 5}, {-3, 5}, {5, 3}, {-5, 3}, {0, 6}, {6, 0},
	{1, 6}, {-1, 6}, {6, 1}, {-6, 1}, {2, 6}, {-2, 6}, {6, 2}, {-6, 2},
	{4, 5}, {-4, 5}, {5, 4}, {-5, 4}, {3, 6}, {-3, 6}, {6, 3}, {-6, 3},
	{0, 7}, {7, 0}, {4, 6}, {-4, 6}, {6, 4}, {-6, 4}, {1, 7}, {-1, 7},
	{5, 5}, {-5, 5}, {7, 1}, {-7, 1}, {2, 7}, {-2, 7}, {7, 2}, {-7, 2},
	{3, 7}, {-3, 7}, {7, 3}, {-7, 3}, {6, 5}, {-6, 5}, {5, 6}, {-5, 6},
	{8, 0}, {4, 7}, {-4, 7}, {7, 4}, {-7, 4}, {8, 1}, {8, 2}, {6, 6},
	{-6, 6}, {8, 3}, {5, 7}, {-5, 7}, {7, 5}, {-7, 5}, {8, 4}, {6, 7},
	{-6, 7}, {7, 6}, {-7, 6}, {8, 5}, {8, 6}, {7, 7}, {-7, 7}, {8, 7},
}

// Transform types.
const (
	transformPredictor  = 0
	transformCrossColor = 1
	transformSubGreen   = 2
	transformPalette    = 3
)

type transform struct {
	kind int
	// For predictor and cross-color: tile size bits and mode map (as G channel).
	tileBits int
	data     []uint32 // decoded as mini VP8L image
	// For palette: list of colors.
	palette []uint32
}

func decodeVP8L(data []byte) (image.Image, error) {
	br := newBitReader(data)

	// Read VP8L magic.
	magic := br.readBits(8)
	if magic != 0x2F {
		return nil, fmt.Errorf("vp8ldec: bad magic 0x%02X", magic)
	}

	// Read dimensions.
	width := int(br.readBits(14)) + 1
	height := int(br.readBits(14)) + 1
	_ = br.readBit() // alpha_hint
	version := br.readBits(3)
	if version != 0 {
		return nil, fmt.Errorf("vp8ldec: unsupported version %d", version)
	}

	// Read transforms.
	var transforms []transform
	for br.readBit() == 1 {
		kind := int(br.readBits(2))
		tr := transform{kind: kind}
		switch kind {
		case transformSubGreen:
			// No data.
		case transformPredictor, transformCrossColor:
			tr.tileBits = int(br.readBits(3)) + 2
			tilesW := (width + (1<<tr.tileBits) - 1) >> tr.tileBits
			tilesH := (height + (1<<tr.tileBits) - 1) >> tr.tileBits
			var err error
			tr.data, err = decodeLiteralImage(br, tilesW, tilesH)
			if err != nil {
				return nil, fmt.Errorf("vp8ldec: transform %d data: %w", kind, err)
			}
		case transformPalette:
			palSize := int(br.readBits(8)) + 1
			// Palette is stored as a VP8L sub-image (delta-encoded).
			deltaPal, err := decodeLiteralImage(br, palSize, 1)
			if err != nil {
				return nil, fmt.Errorf("vp8ldec: palette data: %w", err)
			}
			palette := make([]uint32, palSize)
			palette[0] = deltaPal[0]
			for i := 1; i < palSize; i++ {
				palette[i] = addARGB(palette[i-1], deltaPal[i])
			}
			tr.palette = palette
		}
		transforms = append(transforms, tr)
	}

	// Read the main Huffman trees and pixel data.
	// decodeLiteralImage reads the color-cache bit internally.
	pixels, err := decodeLiteralImage(br, width, height)
	if err != nil {
		return nil, fmt.Errorf("vp8ldec: pixel data: %w", err)
	}

	// Apply transforms in reverse order.
	for i := len(transforms) - 1; i >= 0; i-- {
		tr := &transforms[i]
		switch tr.kind {
		case transformSubGreen:
			applySubGreenInverse(pixels, width, height)
		case transformPredictor:
			applyPredictorInverse(pixels, width, height, tr.tileBits, tr.data)
		case transformCrossColor:
			applyCrossColorInverse(pixels, width, height, tr.tileBits, tr.data)
		case transformPalette:
			pixels, err = applyPaletteInverse(pixels, width, height, tr.palette)
			if err != nil {
				return nil, err
			}
		}
	}

	// Build the output image.
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			p := pixels[y*width+x]
			a := uint8(p >> 24)
			r := uint8(p >> 16)
			g := uint8(p >> 8)
			b := uint8(p)
			img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
		}
	}
	return img, nil
}

// decodeLiteralImage decodes a VP8L pixel stream (used for both the main image
// and transform data images). The color-cache bit is always read here, matching
// the VP8L sub-image format emitted by writeMiniImage.
func decodeLiteralImage(br *bitReader, width, height int) ([]uint32, error) {
	// Read color cache bit (always present in VP8L sub-image format).
	ccBits := 0
	if br.readBit() == 1 {
		ccBits = int(br.readBits(4))
	}
	ccSize := 0
	if ccBits > 0 {
		ccSize = 1 << ccBits
	}
	gAlphabetSize := 256 + 24 + ccSize
	rAlphabetSize := 256
	bAlphabetSize := 256
	aAlphabetSize := 256
	dAlphabetSize := 40

	// Read Huffman trees (always a single meta-group for now).
	gTree, err := readHuffTree(br, gAlphabetSize)
	if err != nil {
		return nil, fmt.Errorf("G tree: %w", err)
	}
	rTree, err := readHuffTree(br, rAlphabetSize)
	if err != nil {
		return nil, fmt.Errorf("R tree: %w", err)
	}
	bTree, err := readHuffTree(br, bAlphabetSize)
	if err != nil {
		return nil, fmt.Errorf("B tree: %w", err)
	}
	aTree, err := readHuffTree(br, aAlphabetSize)
	if err != nil {
		return nil, fmt.Errorf("A tree: %w", err)
	}
	dTree, err := readHuffTree(br, dAlphabetSize)
	if err != nil {
		return nil, fmt.Errorf("D tree: %w", err)
	}

	// Color cache.
	var colorCache []uint32
	if ccBits > 0 {
		colorCache = make([]uint32, 1<<ccBits)
	}
	ccMask := uint32((1 << ccBits) - 1)

	pixels := make([]uint32, width*height)
	i := 0
	for i < width*height {
		sym, err := gTree.readSymbol(br)
		if err != nil {
			return nil, fmt.Errorf("pixel %d G: %w", i, err)
		}

		if sym < 256 {
			// Literal: G channel is sym, read R, B, A.
			gVal := uint8(sym)
			rSym, err := rTree.readSymbol(br)
			if err != nil {
				return nil, err
			}
			bSym, err := bTree.readSymbol(br)
			if err != nil {
				return nil, err
			}
			aSym, err := aTree.readSymbol(br)
			if err != nil {
				return nil, err
			}
			argb := uint32(aSym)<<24 | uint32(rSym)<<16 | uint32(gVal)<<8 | uint32(bSym)
			pixels[i] = argb
			if ccBits > 0 {
				hash := (argb * 0x1e35a7bd) >> (32 - ccBits)
				colorCache[hash&ccMask] = argb
			}
			i++
		} else if sym < 256+24 {
			// Backward reference.
			lengthCode := sym - 256
			length, err := readLengthValue(br, lengthCode)
			if err != nil {
				return nil, err
			}
			distSym, err := dTree.readSymbol(br)
			if err != nil {
				return nil, err
			}
			dist, err := readDistValue(br, distSym, i, width)
			if err != nil {
				return nil, err
			}
			// Copy length pixels from i-dist.
			src := i - dist
			if src < 0 {
				return nil, fmt.Errorf("vp8ldec: backward ref out of bounds: i=%d dist=%d", i, dist)
			}
			for k := 0; k < length && i < width*height; k++ {
				argb := pixels[src+k]
				pixels[i] = argb
				if ccBits > 0 {
					hash := (argb * 0x1e35a7bd) >> (32 - ccBits)
					colorCache[hash&ccMask] = argb
				}
				i++
			}
		} else {
			// Color cache reference.
			ccIdx := sym - 256 - 24
			if ccBits == 0 || ccIdx >= ccSize {
				return nil, fmt.Errorf("vp8ldec: color cache index %d out of range", ccIdx)
			}
			pixels[i] = colorCache[ccIdx]
			i++
		}
	}
	return pixels, nil
}

// readLengthValue decodes a VP8L length value from a length code (0..23).
func readLengthValue(br *bitReader, code int) (int, error) {
	if code < 4 {
		return code + 1, nil
	}
	extraBits := (code - 2) >> 1
	offset := ((2 + (code & 1)) << extraBits) + 1
	extra := int(br.readBits(extraBits))
	return offset + extra, nil
}

// readDistValue decodes a VP8L distance value from a dist code.
func readDistValue(br *bitReader, code, pixelPos, width int) (int, error) {
	if code < 4 {
		return code + 1, nil
	}
	extraBits := (code - 2) >> 1
	offset := ((2 + (code & 1)) << extraBits) + 1
	extra := int(br.readBits(extraBits))
	dist := offset + extra

	// Convert from VP8L distance code to pixel distance.
	if dist <= 120 {
		dx := distTable[dist-1][0]
		dy := distTable[dist-1][1]
		pDist := dy*width + dx
		if pDist < 1 {
			pDist = 1
		}
		return pDist, nil
	}
	return dist - 120, nil
}

// ---- Transform inverses ----

func applySubGreenInverse(pixels []uint32, width, height int) {
	for i, p := range pixels {
		a := uint8(p >> 24)
		r := uint8(p>>16) + uint8(p>>8)
		g := uint8(p >> 8)
		b := uint8(p) + uint8(p>>8)
		pixels[i] = uint32(a)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
	}
}

func applyPredictorInverse(pixels []uint32, width, height, tileBits int, tileData []uint32) {
	tileW := (width + (1<<tileBits) - 1) >> tileBits

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if x == 0 && y == 0 {
				pixels[0] = addARGB(pixels[0], 0xFF000000)
				continue
			}
			var mode int
			if y == 0 {
				mode = 1 // top row: left predictor
			} else if x == 0 {
				mode = 2 // left column: top predictor
			} else {
				tx := x >> tileBits
				ty := y >> tileBits
				mode = int(tileData[ty*tileW+tx]>>8) & 0xFF
			}
			pred := getPrediction(mode, x, y, width, pixels)
			pixels[idx] = addARGB(pixels[idx], pred)
		}
	}
}

func getPrediction(mode, x, y, width int, pixels []uint32) uint32 {
	left := func() uint32 {
		if x > 0 {
			return pixels[y*width+x-1]
		}
		return 0xFF000000
	}
	top := func() uint32 {
		if y > 0 {
			return pixels[(y-1)*width+x]
		}
		return 0xFF000000
	}
	topLeft := func() uint32 {
		if y > 0 && x > 0 {
			return pixels[(y-1)*width+x-1]
		}
		return 0xFF000000
	}
	topRight := func() uint32 {
		if y > 0 && x+1 < width {
			return pixels[(y-1)*width+x+1]
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

func average2(a, b uint32) uint32 {
	return (((a ^ b) & 0xFEFEFEFE) >> 1) + (a & b)
}

func selectPredictor(left, top, topLeft uint32) uint32 {
	pLeft := int32(abs8(top>>24, topLeft>>24) + abs8(top>>16, topLeft>>16) +
		abs8(top>>8, topLeft>>8) + abs8(top, topLeft))
	pTop := int32(abs8(left>>24, topLeft>>24) + abs8(left>>16, topLeft>>16) +
		abs8(left>>8, topLeft>>8) + abs8(left, topLeft))
	if pLeft <= pTop {
		return left
	}
	return top
}

func abs8(a, b uint32) int32 {
	d := int32(a&0xFF) - int32(b&0xFF)
	if d < 0 {
		return -d
	}
	return d
}

func clampAddSubFull(left, top, topLeft uint32) uint32 {
	return clampedAdd4(left, top, ^topLeft+1)
}

func clampAddSubHalf(avg, topLeft uint32) uint32 {
	ra := clamp8i(int32(avg>>24&0xFF) + (int32(avg>>24&0xFF)-int32(topLeft>>24&0xFF))/2)
	rr := clamp8i(int32(avg>>16&0xFF) + (int32(avg>>16&0xFF)-int32(topLeft>>16&0xFF))/2)
	rg := clamp8i(int32(avg>>8&0xFF) + (int32(avg>>8&0xFF)-int32(topLeft>>8&0xFF))/2)
	rb := clamp8i(int32(avg&0xFF) + (int32(avg&0xFF)-int32(topLeft&0xFF))/2)
	return ra<<24 | rr<<16 | rg<<8 | rb
}

func clampedAdd4(a, b, c uint32) uint32 {
	ra := clamp8i(int32(a>>24&0xFF) + int32(b>>24&0xFF) + int32(c>>24&0xFF))
	rr := clamp8i(int32(a>>16&0xFF) + int32(b>>16&0xFF) + int32(c>>16&0xFF))
	rg := clamp8i(int32(a>>8&0xFF) + int32(b>>8&0xFF) + int32(c>>8&0xFF))
	rb := clamp8i(int32(a&0xFF) + int32(b&0xFF) + int32(c&0xFF))
	return ra<<24 | rr<<16 | rg<<8 | rb
}

func clamp8i(v int32) uint32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint32(v)
}

func applyCrossColorInverse(pixels []uint32, width, height, tileBits int, tileData []uint32) {
	tileW := (width + (1<<tileBits) - 1) >> tileBits

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			tx := x >> tileBits
			ty := y >> tileBits
			m := tileData[ty*tileW+tx]
			greenToRed := int8(m >> 16)
			greenToBlue := int8(m)

			p := pixels[y*width+x]
			g := int32(uint8(p >> 8))
			r := uint8(p>>16) + uint8(int32(greenToRed)*g>>5)
			b := uint8(p) + uint8(int32(greenToBlue)*g>>5)
			pixels[y*width+x] = (p & 0xFF00FF00) | uint32(r)<<16 | uint32(b)
		}
	}
}

func applyPaletteInverse(pixels []uint32, width, height int, palette []uint32) ([]uint32, error) {
	palSize := len(palette)
	bits := 0
	for (1 << bits) < palSize {
		bits++
	}
	if bits == 0 {
		bits = 1
	}
	pixPerByte := 8 / bits
	_ = pixPerByte

	out := make([]uint32, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			packed := pixels[y*width+x]
			idx := int(packed >> 8 & 0xFF)
			if idx >= palSize {
				return nil, fmt.Errorf("vp8ldec: palette index %d >= palette size %d", idx, palSize)
			}
			out[y*width+x] = palette[idx]
		}
	}
	return out, nil
}

func max1(a int) int {
	if a < 1 {
		return 1
	}
	return a
}
