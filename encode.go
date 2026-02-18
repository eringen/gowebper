// Package gowebper encodes images to lossless WebP (VP8L) format.
// It is a pure Go implementation with no external dependencies.
package gowebper

import (
	"bytes"
	"encoding/binary"
	"image"
	"io"

	"github.com/eringen/gowebper/internal/bitwriter"
	"github.com/eringen/gowebper/internal/colormodel"
	"github.com/eringen/gowebper/internal/huffman"
	"github.com/eringen/gowebper/internal/lz77"
	"github.com/eringen/gowebper/internal/transform"
)

// Level constants for convenience.
const (
	LevelFastest = 0
	LevelDefault = 6
	LevelBest    = 9
)

// Options configures the encoder.
type Options struct {
	// Level is a compression effort in [0, 9].
	// 0 = fastest, largest file. 9 = slowest, smallest file.
	// The zero value (0) means LevelFastest, not LevelDefault. To get the
	// default level use LevelDefault or a nil *Options.
	Level int
}

// Encode writes m to w as a lossless WebP (VP8L) file.
// If opts is nil a default level (6) is used.
func Encode(w io.Writer, m image.Image, opts *Options) error {
	enc := NewEncoder(opts)
	return enc.Encode(w, m)
}

// EncodeToBytes encodes m and returns the WebP bytes.
func EncodeToBytes(m image.Image, opts *Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := Encode(&buf, m, opts); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Encoder amortises allocations across multiple calls.
type Encoder struct {
	opts    Options
	scratch []uint32
}

// NewEncoder returns an Encoder configured by opts.
// If opts is nil, LevelDefault is used.
func NewEncoder(opts *Options) *Encoder {
	e := &Encoder{}
	if opts != nil {
		e.opts = *opts
	} else {
		e.opts.Level = LevelDefault
	}
	if e.opts.Level < 0 {
		e.opts.Level = 0
	}
	if e.opts.Level > 9 {
		e.opts.Level = 9
	}
	return e
}

// Encode encodes m and writes the WebP bytes to w.
func (e *Encoder) Encode(w io.Writer, m image.Image) error {
	b := m.Bounds()
	width := b.Max.X - b.Min.X
	height := b.Max.Y - b.Min.Y
	if width <= 0 || height <= 0 {
		return nil
	}

	// Convert image to ARGB pixels.
	pixels := colormodel.ToARGB(m)
	hasAlpha := colormodel.HasAlpha(pixels)

	// Encode the VP8L bitstream.
	vp8lBytes, err := e.encodeVP8L(pixels, width, height, hasAlpha)
	if err != nil {
		return err
	}

	// Write RIFF container.
	return writeRIFF(w, vp8lBytes)
}

// writeRIFF writes the RIFF/WEBP container around the VP8L chunk.
func writeRIFF(w io.Writer, vp8lData []byte) error {
	// Pad to even length.
	padded := len(vp8lData)
	if padded%2 != 0 {
		padded++
	}
	totalSize := 4 + 4 + 4 + padded // "WEBP" + "VP8L" + len(uint32) + data
	riff := make([]byte, 8+totalSize)
	copy(riff[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riff[4:8], uint32(totalSize))
	copy(riff[8:12], "WEBP")
	copy(riff[12:16], "VP8L")
	binary.LittleEndian.PutUint32(riff[16:20], uint32(len(vp8lData)))
	copy(riff[20:], vp8lData)
	_, err := w.Write(riff)
	return err
}

// levelConfig holds the per-level algorithm parameters.
type levelConfig struct {
	useSubGreen   bool
	usePredictor  bool
	useCrossColor bool
	predTileBits  int // Predictor transform tile bits
	ccTileBits    int // CrossColor transform tile bits
	lz77Window    int // LZ77 window size in pixels (0 = disabled)
	chainDepth    int // LZ77 hash chain depth
	ccBits        int // Color cache bits (0 = disabled)
}

var levelConfigs = [10]levelConfig{
	0: {false, false, false, 0, 0, 0, 0, 0},
	1: {true, false, false, 0, 0, 1 << 10, 1, 0},
	2: {true, false, false, 0, 0, 1 << 11, 2, 0},
	3: {true, false, false, 0, 0, 1 << 12, 4, 0},
	4: {true, true, false, 4, 0, 1 << 13, 8, 0},
	5: {true, true, false, 3, 0, 1 << 14, 16, 0},
	6: {true, true, false, 3, 0, 1 << 15, 32, 0},
	7: {true, true, true, 2, 3, 1 << 16, 64, 0},
	8: {true, true, true, 2, 2, 1 << 18, 256, 6},
	9: {true, true, true, 2, 2, 1 << 20, 1024, 8},
}

// encodeVP8L produces the VP8L bitstream (without RIFF wrapper).
func (e *Encoder) encodeVP8L(pixels []uint32, width, height int, hasAlpha bool) ([]byte, error) {
	cfg := levelConfigs[e.opts.Level]

	var buf bytes.Buffer
	bw := bitwriter.New(&buf)

	// VP8L header.
	bw.WriteBits(0x2F, 8)
	bw.WriteBits(uint32(width-1), 14)
	bw.WriteBits(uint32(height-1), 14)
	alphaHint := uint32(0)
	if hasAlpha {
		alphaHint = 1
	}
	bw.WriteBits(alphaHint, 1)
	bw.WriteBits(0, 3) // version = 0

	// Work on a copy of pixels for transforms.
	work := make([]uint32, len(pixels))
	copy(work, pixels)

	// Attempt palette transform at all levels.
	palApplied, palette := tryPaletteTransform(bw, work, width, height)
	if palApplied {
		// After palette transform, image is index-mapped.
		// The "width" for further encoding uses the packed index stream.
		// For simplicity we encode the index stream directly as a VP8L image.
		// The pixel data in work[] has indices in the G channel.
		// Terminate transforms.
		bw.WriteBits(0, 1)

		// No color cache.
		bw.WriteBits(0, 1)

		// Encode index stream with Huffman (no LZ77 for palette mode now).
		encodePixelStream(bw, work, width, height, 0, nil)

		_ = palette
	} else {
		// Apply transforms according to level.
		if cfg.useSubGreen {
			bw.WriteBits(1, 1) // more transforms
			bw.WriteBits(transformSubGreen, 2)
			transform.SubtractGreenForward(work)
		}

		if cfg.usePredictor {
			bw.WriteBits(1, 1)
			bw.WriteBits(transformPredictor, 2)
			writePredictorTransform(bw, work, width, height, cfg.predTileBits)
		}

		if cfg.useCrossColor && !palApplied {
			bw.WriteBits(1, 1)
			bw.WriteBits(transformCrossColor, 2)
			writeCrossColorTransform(bw, work, width, height, cfg.ccTileBits)
		}

		// End of transforms.
		bw.WriteBits(0, 1)

		// Color cache.
		ccBits := cfg.ccBits
		if ccBits > 0 {
			bw.WriteBits(1, 1)
			bw.WriteBits(uint32(ccBits), 4)
		} else {
			bw.WriteBits(0, 1)
		}

		// Tokenise and encode pixel stream.
		var tokens []lz77.Token
		if cfg.lz77Window > 0 {
			tokens = lz77.Tokenise(work, width, cfg.lz77Window, cfg.chainDepth, ccBits)
		}
		encodePixelStream(bw, work, width, height, ccBits, tokens)
	}

	if err := bw.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Transform type codes for VP8L bitstream.
const (
	transformPredictor  = 0
	transformCrossColor = 1
	transformSubGreen   = 2
	transformPalette    = 3
)

// encodePixelStream writes Huffman trees and the token stream.
func encodePixelStream(bw *bitwriter.BitWriter, pixels []uint32, width, height, ccBits int, tokens []lz77.Token) {
	ccSize := 0
	if ccBits > 0 {
		ccSize = 1 << ccBits
	}
	gAlphabetSize := 256 + 24 + ccSize

	// Build frequency tables.
	gBuilder := huffman.NewBuilder(gAlphabetSize)
	rBuilder := huffman.NewBuilder(256)
	bBuilder := huffman.NewBuilder(256)
	aBuilder := huffman.NewBuilder(256)
	dBuilder := huffman.NewBuilder(40)

	if len(tokens) > 0 {
		// Accumulate from token stream.
		for _, tok := range tokens {
			if tok.IsLiteral() {
				p := tok.Pixel()
				gBuilder.Add(int(p >> 8 & 0xFF))
				rBuilder.Add(int(p >> 16 & 0xFF))
				bBuilder.Add(int(p & 0xFF))
				aBuilder.Add(int(p >> 24))
			} else if tok.IsColorCache() {
				gBuilder.Add(256 + 24 + tok.CacheIndex())
			} else {
				lc, dc := tok.LengthCode(), tok.DistCode()
				gBuilder.Add(256 + lc)
				dBuilder.Add(dc)
			}
		}
	} else {
		// All literals.
		for _, p := range pixels {
			gBuilder.Add(int(p >> 8 & 0xFF))
			rBuilder.Add(int(p >> 16 & 0xFF))
			bBuilder.Add(int(p & 0xFF))
			aBuilder.Add(int(p >> 24))
		}
	}
	// Ensure every tree has at least one symbol (VP8L requires it).
	// We always add sym 0 once; this is harmless if sym 0 was already present.
	gBuilder.Add(0)
	rBuilder.Add(0)
	bBuilder.Add(0)
	aBuilder.Add(0)
	dBuilder.Add(0)

	gTree := gBuilder.Build()
	rTree := rBuilder.Build()
	bTree := bBuilder.Build()
	aTree := aBuilder.Build()
	dTree := dBuilder.Build()

	gTree.WriteTo(bw)
	rTree.WriteTo(bw)
	bTree.WriteTo(bw)
	aTree.WriteTo(bw)
	dTree.WriteTo(bw)

	// Write token stream.
	if len(tokens) > 0 {
		writeTokenStream(bw, tokens, gTree, rTree, bTree, aTree, dTree)
	} else {
		// Write literal pixels.
		for _, p := range pixels {
			g := int(p >> 8 & 0xFF)
			r := int(p >> 16 & 0xFF)
			bv := int(p & 0xFF)
			a := int(p >> 24)
			bw.WriteBits(gTree.Codes[g], gTree.Lengths[g])
			bw.WriteBits(rTree.Codes[r], rTree.Lengths[r])
			bw.WriteBits(bTree.Codes[bv], bTree.Lengths[bv])
			bw.WriteBits(aTree.Codes[a], aTree.Lengths[a])
		}
	}
}


// writeTokenStream writes an LZ77+literal token stream using the provided Huffman trees.
func writeTokenStream(bw *bitwriter.BitWriter, tokens []lz77.Token,
	gTree, rTree, bTree, aTree, dTree *huffman.Tree) {

	for _, tok := range tokens {
		if tok.IsLiteral() {
			p := tok.Pixel()
			g := int(p >> 8 & 0xFF)
			r := int(p >> 16 & 0xFF)
			bv := int(p & 0xFF)
			a := int(p >> 24)
			bw.WriteBits(gTree.Codes[g], gTree.Lengths[g])
			bw.WriteBits(rTree.Codes[r], rTree.Lengths[r])
			bw.WriteBits(bTree.Codes[bv], bTree.Lengths[bv])
			bw.WriteBits(aTree.Codes[a], aTree.Lengths[a])
		} else if tok.IsColorCache() {
			idx := tok.CacheIndex()
			sym := 256 + 24 + idx
			bw.WriteBits(gTree.Codes[sym], gTree.Lengths[sym])
		} else {
			// Backward reference.
			lc, le := tok.LengthCode(), tok.LengthExtra()
			dc, de, deBits := tok.DistCode(), tok.DistExtra(), tok.DistExtraBits()

			sym := 256 + lc
			bw.WriteBits(gTree.Codes[sym], gTree.Lengths[sym])
			bw.WriteBits(uint32(le), lz77.LengthExtraBits(lc))
			bw.WriteBits(dTree.Codes[dc], dTree.Lengths[dc])
			bw.WriteBits(uint32(de), deBits)
		}
	}
}

// tryPaletteTransform attempts to apply the palette transform.
// It returns true and emits the transform header if the image uses <= 256 colors.
func tryPaletteTransform(bw *bitwriter.BitWriter, pixels []uint32, width, height int) (bool, []uint32) {
	// Collect distinct colors.
	colorMap := make(map[uint32]int)
	for _, p := range pixels {
		if len(colorMap) > 256 {
			return false, nil
		}
		if _, ok := colorMap[p]; !ok {
			colorMap[p] = len(colorMap)
		}
	}
	if len(colorMap) > 256 {
		return false, nil
	}

	// Build sorted palette.
	palette := make([]uint32, len(colorMap))
	for color, idx := range colorMap {
		palette[idx] = color
	}

	// Emit palette transform header.
	bw.WriteBits(1, 1) // more transforms
	bw.WriteBits(transformPalette, 2)
	bw.WriteBits(uint32(len(palette)-1), 8) // palette size - 1

	// Delta-encode palette and store as a VP8L sub-image.
	delta := make([]uint32, len(palette))
	delta[0] = palette[0]
	for i := 1; i < len(palette); i++ {
		p, q := palette[i], palette[i-1]
		da := uint8(p>>24) - uint8(q>>24)
		dr := uint8(p>>16) - uint8(q>>16)
		dg := uint8(p>>8) - uint8(q>>8)
		db := uint8(p) - uint8(q)
		delta[i] = uint32(da)<<24 | uint32(dr)<<16 | uint32(dg)<<8 | uint32(db)
	}
	writeMiniImage(bw, delta, len(palette), 1)

	// Replace pixels with palette indices (in G channel).
	for i, p := range pixels {
		idx := colorMap[p]
		pixels[i] = uint32(idx) << 8
	}

	return true, palette
}

// writePredictorTransform applies the predictor transform and emits its header.
func writePredictorTransform(bw *bitwriter.BitWriter, pixels []uint32, width, height, tileBits int) {
	bw.WriteBits(uint32(tileBits-2), 3)

	tileW := (width + (1<<tileBits) - 1) >> tileBits
	tileH := (height + (1<<tileBits) - 1) >> tileBits
	tileModes := transform.BuildPredictorModes(pixels, width, height, tileBits)

	// Encode tile modes as a mini VP8L image (G channel holds mode 0..13).
	tilePix := make([]uint32, tileW*tileH)
	for i, mode := range tileModes {
		tilePix[i] = 0xFF000000 | uint32(mode)<<8
	}

	// Write tile image as literal-only sub-image.
	writeMiniImage(bw, tilePix, tileW, tileH)

	// Apply the predictor transform forward.
	transform.PredictorForward(pixels, width, height, tileBits, tileModes)
}

// writeCrossColorTransform applies the cross-color transform and emits its header.
func writeCrossColorTransform(bw *bitwriter.BitWriter, pixels []uint32, width, height, tileBits int) {
	bw.WriteBits(uint32(tileBits-2), 3)

	tileW := (width + (1<<tileBits) - 1) >> tileBits
	tileH := (height + (1<<tileBits) - 1) >> tileBits
	tileMultipliers := transform.BuildCrossColorMultipliers(pixels, width, height, tileBits)

	// Encode multiplier map as mini VP8L image.
	tilePix := make([]uint32, tileW*tileH)
	for i, m := range tileMultipliers {
		// m is packed as (greenToRed<<16 | greenToBlue) in the ARGB word.
		tilePix[i] = 0xFF000000 | m
	}

	writeMiniImage(bw, tilePix, tileW, tileH)

	// Apply cross-color transform forward.
	transform.CrossColorForward(pixels, width, height, tileBits, tileMultipliers)
}

// writeMiniImage writes a small ARGB image as a literal-only VP8L sub-image
// (used for transform data images). No color cache, no transforms, no LZ77.
func writeMiniImage(bw *bitwriter.BitWriter, pixels []uint32, width, height int) {
	// No color cache.
	bw.WriteBits(0, 1)

	// Build Huffman trees.
	gBuilder := huffman.NewBuilder(256 + 24)
	rBuilder := huffman.NewBuilder(256)
	bBuilder := huffman.NewBuilder(256)
	aBuilder := huffman.NewBuilder(256)
	dBuilder := huffman.NewBuilder(40)

	for _, p := range pixels {
		gBuilder.Add(int(p >> 8 & 0xFF))
		rBuilder.Add(int(p >> 16 & 0xFF))
		bBuilder.Add(int(p & 0xFF))
		aBuilder.Add(int(p >> 24))
	}
	dBuilder.Add(0)

	gTree := gBuilder.Build()
	rTree := rBuilder.Build()
	bTree := bBuilder.Build()
	aTree := aBuilder.Build()
	dTree := dBuilder.Build()

	gTree.WriteTo(bw)
	rTree.WriteTo(bw)
	bTree.WriteTo(bw)
	aTree.WriteTo(bw)
	dTree.WriteTo(bw)

	for _, p := range pixels {
		g := int(p >> 8 & 0xFF)
		r := int(p >> 16 & 0xFF)
		bv := int(p & 0xFF)
		a := int(p >> 24)
		bw.WriteBits(gTree.Codes[g], gTree.Lengths[g])
		bw.WriteBits(rTree.Codes[r], rTree.Lengths[r])
		bw.WriteBits(bTree.Codes[bv], bTree.Lengths[bv])
		bw.WriteBits(aTree.Codes[a], aTree.Lengths[a])
	}
}
