package vp8ldec

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/eringen/gowebper/internal/bitwriter"
	"github.com/eringen/gowebper/internal/huffman"
)

// buildSimpleVP8L builds a minimal VP8L bitstream for a w×h image containing
// the given ARGB pixels using literal-only encoding (no transforms, no LZ77).
func buildSimpleVP8L(t *testing.T, w, h int, pixels []uint32) []byte {
	t.Helper()

	var buf bytes.Buffer
	bw := bitwriter.New(&buf)

	// VP8L header.
	bw.WriteBits(0x2F, 8) // magic
	bw.WriteBits(uint32(w-1), 14)
	bw.WriteBits(uint32(h-1), 14)
	bw.WriteBits(0, 1) // alpha_hint
	bw.WriteBits(0, 3) // version

	// No transforms.
	bw.WriteBits(0, 1)

	// No color cache.
	bw.WriteBits(0, 1)

	// No meta-Huffman (use_meta = 0, level 0 only).
	bw.WriteBits(0, 1)

	// Build Huffman trees from pixel data.
	gBuilder := huffman.NewBuilder(256 + 24)
	rBuilder := huffman.NewBuilder(256)
	bBuilder := huffman.NewBuilder(256)
	aBuilder := huffman.NewBuilder(256)
	dBuilder := huffman.NewBuilder(40)

	for _, p := range pixels {
		a := int(p >> 24)
		r := int(p >> 16 & 0xFF)
		g := int(p >> 8 & 0xFF)
		bv := int(p & 0xFF)
		gBuilder.Add(g)
		rBuilder.Add(r)
		bBuilder.Add(bv)
		aBuilder.Add(a)
	}
	// Ensure at least one symbol in distance tree.
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

	// VP8L single-symbol trees consume 0 bits per read.
	gTree.ZeroSingleSymbol()
	rTree.ZeroSingleSymbol()
	bTree.ZeroSingleSymbol()
	aTree.ZeroSingleSymbol()
	dTree.ZeroSingleSymbol()

	// Write literal pixels.
	for _, p := range pixels {
		a := int(p >> 24)
		r := int(p >> 16 & 0xFF)
		g := int(p >> 8 & 0xFF)
		bv := int(p & 0xFF)
		// G symbol (literal codes 0..255).
		bw.WriteBits(gTree.Codes[g], gTree.Lengths[g])
		bw.WriteBits(rTree.Codes[r], rTree.Lengths[r])
		bw.WriteBits(bTree.Codes[bv], bTree.Lengths[bv])
		bw.WriteBits(aTree.Codes[a], aTree.Lengths[a])
	}
	if err := bw.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Wrap in RIFF container.
	vp8lData := buf.Bytes()
	riff := make([]byte, 20+len(vp8lData))
	copy(riff[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riff[4:8], uint32(len(riff)-8))
	copy(riff[8:12], "WEBP")
	copy(riff[12:16], "VP8L")
	binary.LittleEndian.PutUint32(riff[16:20], uint32(len(vp8lData)))
	copy(riff[20:], vp8lData)
	return riff
}

func TestDecode_solidRed(t *testing.T) {
	pixels := []uint32{0xFFFF0000} // 1x1 solid red (non-premultiplied ARGB)
	data := buildSimpleVP8L(t, 1, 1, pixels)

	img, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Errorf("expected 1x1, got %v", img.Bounds())
	}
	r, g, b, a := img.At(0, 0).RGBA()
	// img is NRGBA; RGBA() returns premultiplied 16-bit values.
	// For NRGBA with A=255, R=255: RGBA() returns (65535, 0, 0, 65535).
	if a != 65535 || r != 65535 || g != 0 || b != 0 {
		t.Errorf("pixel: r=%d g=%d b=%d a=%d, want 65535,0,0,65535", r, g, b, a)
	}
}

func TestDecode_2x2(t *testing.T) {
	pixels := []uint32{
		0xFFFF0000, // (0,0) red
		0xFF00FF00, // (1,0) green
		0xFF0000FF, // (0,1) blue
		0xFFFFFFFF, // (1,1) white
	}
	data := buildSimpleVP8L(t, 2, 2, pixels)

	img, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("expected 2x2, got %v", b)
	}
	// Verify corner pixels.
	checkPixel := func(x, y int, wantARGB uint32) {
		t.Helper()
		r, g, bv, a := img.At(x, y).RGBA()
		gotARGB := (uint32(a>>8) << 24) | (uint32(r>>8) << 16) | (uint32(g>>8) << 8) | uint32(bv>>8)
		if gotARGB != wantARGB {
			t.Errorf("pixel (%d,%d): got 0x%08X, want 0x%08X", x, y, gotARGB, wantARGB)
		}
	}
	checkPixel(0, 0, 0xFFFF0000)
	checkPixel(1, 0, 0xFF00FF00)
	checkPixel(0, 1, 0xFF0000FF)
	checkPixel(1, 1, 0xFFFFFFFF)
}

func TestDecode_badMagic(t *testing.T) {
	data := []byte("RIFF\x10\x00\x00\x00WEBPVP8L\x08\x00\x00\x00\xAA\x00\x00\x00\x00\x00\x00\x00")
	_, err := DecodeBytes(data)
	if err == nil {
		t.Error("expected error for bad magic, got nil")
	}
}

func TestDecode_transparent(t *testing.T) {
	pixels := []uint32{0x80FF8040} // semi-transparent pixel
	data := buildSimpleVP8L(t, 1, 1, pixels)

	img, err := DecodeBytes(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Check using direct NRGBA access.
	type nrgbaer interface {
		NRGBAAt(x, y int) interface{ RGBA() (uint32, uint32, uint32, uint32) }
	}
	r, g, bv, a := img.At(0, 0).RGBA()
	_ = r
	_ = g
	_ = bv
	// The alpha should be approximately 0x80/0xFF * 65535 ≈ 32896.
	// But since we store non-premultiplied, a/65535 * 255 should ≈ 0x80.
	aVal := uint8(a >> 8)
	if aVal != 0x80 {
		t.Errorf("alpha: got 0x%02X, want 0x80", aVal)
	}
}
