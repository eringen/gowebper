//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/eringen/gowebper/internal/bitwriter"
	"github.com/eringen/gowebper/internal/huffman"
)

// Build a minimal 1x1 red VP8L with no transforms (no palette).
func buildMinimal1x1() []byte {
	var buf [1024]byte
	w := &sliceWriter{buf: buf[:0]}
	bw := bitwriter.New(w)

	// VP8L header
	bw.WriteBits(0x2F, 8)  // signature
	bw.WriteBits(0, 14)    // width-1 = 0
	bw.WriteBits(0, 14)    // height-1 = 0
	bw.WriteBits(0, 1)     // alpha_is_used = 0
	bw.WriteBits(0, 3)     // version = 0

	// No transforms
	bw.WriteBits(0, 1)

	// No color cache
	bw.WriteBits(0, 1)

	// 1 pixel: ARGB = (255, 255, 0, 0) = opaque red
	// G=0, R=255, B=0, A=255
	// G tree: 1 symbol (0), alphabet 280
	// R tree: 1 symbol (255), alphabet 256
	// B tree: 1 symbol (0), alphabet 256
	// A tree: 1 symbol (255), alphabet 256
	// D tree: 1 symbol (0), alphabet 40

	// Build trees
	gb := huffman.NewBuilder(280)
	gb.Add(0)
	gTree := gb.Build()

	rb := huffman.NewBuilder(256)
	rb.Add(255)
	rTree := rb.Build()

	bb := huffman.NewBuilder(256)
	bb.Add(0)
	bTree := bb.Build()

	ab := huffman.NewBuilder(256)
	ab.Add(255)
	aTree := ab.Build()

	db := huffman.NewBuilder(40)
	db.Add(0)
	dTree := db.Build()

	gTree.WriteTo(bw)
	rTree.WriteTo(bw)
	bTree.WriteTo(bw)
	aTree.WriteTo(bw)
	dTree.WriteTo(bw)

	gTree.ZeroSingleSymbol()
	rTree.ZeroSingleSymbol()
	bTree.ZeroSingleSymbol()
	aTree.ZeroSingleSymbol()
	dTree.ZeroSingleSymbol()

	// 1 pixel of data
	bw.WriteBits(gTree.Codes[0], gTree.Lengths[0])
	bw.WriteBits(rTree.Codes[255], rTree.Lengths[255])
	bw.WriteBits(bTree.Codes[0], bTree.Lengths[0])
	bw.WriteBits(aTree.Codes[255], aTree.Lengths[255])

	bw.Flush()

	vp8l := w.buf
	// Pad VP8L data to avoid libwebp's bitreader eos issue with small files.
	// The VP8L bitreader sets eos when all input bytes are consumed during
	// ShiftBytes, even if enough bits remain in the buffer.
	for len(vp8l) < 20 {
		vp8l = append(vp8l, 0)
	}
	return wrapRIFF(vp8l)
}

func wrapRIFF(vp8l []byte) []byte {
	padded := len(vp8l)
	if padded%2 != 0 {
		padded++
	}
	totalSize := 4 + 4 + 4 + padded
	riff := make([]byte, 8+totalSize)
	copy(riff[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riff[4:8], uint32(totalSize))
	copy(riff[8:12], "WEBP")
	copy(riff[12:16], "VP8L")
	binary.LittleEndian.PutUint32(riff[16:20], uint32(len(vp8l)))
	copy(riff[20:], vp8l)
	return riff
}

type sliceWriter struct {
	buf []byte
}

func (sw *sliceWriter) Write(p []byte) (int, error) {
	sw.buf = append(sw.buf, p...)
	return len(p), nil
}

func main() {
	data := buildMinimal1x1()
	os.WriteFile("test_manual.webp", data, 0644)
	fmt.Printf("Wrote %d bytes\n", len(data))
	fmt.Printf("VP8L data: %d bytes\n", len(data)-20)

	// Hex dump
	for i, b := range data {
		if i%16 == 0 && i > 0 {
			fmt.Println()
		}
		fmt.Printf("%02x ", b)
	}
	fmt.Println()

	// Test with dwebp
	cmd := exec.Command("dwebp", "test_manual.webp", "-o", "/tmp/test_manual.png")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("dwebp failed: %s\n%s", err, out)
	} else {
		fmt.Println("dwebp OK!")
	}
}
