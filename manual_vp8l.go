//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
)

// Manually build a VP8L bitstream using a bit writer.
type bw struct {
	data  []byte
	bits  uint64
	nBits int
}

func (b *bw) write(val uint32, n int) {
	b.bits |= uint64(val) << b.nBits
	b.nBits += n
	for b.nBits >= 8 {
		b.data = append(b.data, byte(b.bits))
		b.bits >>= 8
		b.nBits -= 8
	}
}

func (b *bw) flush() {
	if b.nBits > 0 {
		b.data = append(b.data, byte(b.bits))
		b.bits = 0
		b.nBits = 0
	}
}

func main() {
	// Test 1: Copy the reference VP8L byte-by-byte to verify our RIFF wrapper
	testRewrap()

	// Test 2: Manual no-palette 1x1 red with simple trees
	testSimple()

	// Test 3: Manual no-palette 1x1 red with normal trees (matching reference style)
	testNormal()

	// Test 4: Manual palette 1x1 red (matching reference)
	testPalette()
}

func testRewrap() {
	fmt.Println("=== Test 1: Rewrapped reference ===")
	ref, _ := os.ReadFile("/tmp/red1x1_ref.webp")
	vp8lLen := binary.LittleEndian.Uint32(ref[16:20])
	vp8l := ref[20 : 20+vp8lLen]
	test("test_t1.webp", vp8l)
}

func testSimple() {
	fmt.Println("\n=== Test 2: No palette, simple trees, 1x1 red ===")
	b := &bw{}

	// Header
	b.write(0x2F, 8)  // sig
	b.write(0, 14)    // w-1
	b.write(0, 14)    // h-1
	b.write(0, 1)     // alpha
	b.write(0, 3)     // version

	// No transforms
	b.write(0, 1)
	// No color cache
	b.write(0, 1)

	// G tree: simple, 1 sym, is8=0, sym=0
	b.write(1, 1) // simple
	b.write(0, 1) // 1 sym
	b.write(0, 1) // is8=0
	b.write(0, 1) // sym=0

	// R tree: simple, 1 sym, is8=1, sym=255
	b.write(1, 1)
	b.write(0, 1)
	b.write(1, 1) // is8=1
	b.write(255, 8)

	// B tree: simple, 1 sym, is8=0, sym=0
	b.write(1, 1)
	b.write(0, 1)
	b.write(0, 1)
	b.write(0, 1)

	// A tree: simple, 1 sym, is8=1, sym=255
	b.write(1, 1)
	b.write(0, 1)
	b.write(1, 1)
	b.write(255, 8)

	// D tree: simple, 1 sym, is8=0, sym=0
	b.write(1, 1)
	b.write(0, 1)
	b.write(0, 1)
	b.write(0, 1)

	// No pixel data needed (single-symbol trees, 0 bits per read)
	b.flush()

	test("test_t2.webp", b.data)
}

func testNormal() {
	fmt.Println("\n=== Test 3: No palette, normal trees, 1x1 red ===")
	b := &bw{}

	// Header
	b.write(0x2F, 8)
	b.write(0, 14)
	b.write(0, 14)
	b.write(0, 1)
	b.write(0, 3)

	// No transforms
	b.write(0, 1)
	// No color cache
	b.write(0, 1)

	// Write 5 normal trees, each with 1 symbol
	writeNormalSingleSym(b, 0, 280)   // G: sym 0
	writeNormalSingleSym(b, 255, 256) // R: sym 255
	writeNormalSingleSym(b, 0, 256)   // B: sym 0
	writeNormalSingleSym(b, 255, 256) // A: sym 255
	writeNormalSingleSym(b, 0, 40)    // D: sym 0

	// 1 pixel - 0 bits needed (single-symbol)
	b.flush()
	test("test_t3.webp", b.data)
}

// writeNormalSingleSym writes a normal Huffman tree with exactly 1 symbol.
// The only code_length value is 1 (at position sym). All others are 0.
// CL sequence: repeat 0 for sym positions, then literal 1, then repeat 0 for the rest.
func writeNormalSingleSym(b *bw, sym, alphabetSize int) {
	b.write(0, 1) // simple_code = 0

	// We need CL symbols: 17 (repeat 0, 3-10) and 18 (repeat 0, 11-138)
	// and symbol 1 (literal code length 1).
	// CL code lengths for our 3 symbols:
	// CL sym 1: len 2
	// CL sym 17: len 2
	// CL sym 18: len 1
	// Other CL syms: len 0

	// codeLenOrder = {17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	// We need to emit CL lengths for: idx0=17, idx1=18, idx2=0, idx3=1
	// clLengths[17]=2, clLengths[18]=1, clLengths[0]=0, clLengths[1]=2
	// That's 4 entries (numCLCodes-4=0)

	b.write(0, 4) // numCLCodes - 4 = 0 → 4 entries
	b.write(2, 3) // CL sym 17: len 2
	b.write(1, 3) // CL sym 18: len 1
	b.write(0, 3) // CL sym 0: len 0
	b.write(2, 3) // CL sym 1: len 2

	// max_symbol flag
	b.write(0, 1) // use full alphabet

	// Now encode the code length sequence using the CL tree:
	// CL tree canonical codes:
	// sym 18 (len 1): code 0
	// sym 1 (len 2): code 10 (reversed: 01)
	// sym 17 (len 2): code 11 (reversed: 11)

	// Code length sequence for the alphabet:
	// Positions 0..sym-1: all zeros (sym positions before our symbol)
	// Position sym: code_length=1
	// Positions sym+1..alphabetSize-1: all zeros

	// Encode leading zeros (sym positions)
	remaining := sym
	for remaining >= 11 {
		n := remaining
		if n > 138 {
			n = 138
		}
		b.write(0, 1) // CL sym 18, code=0, 1 bit
		b.write(uint32(n-11), 7)
		remaining -= n
	}
	if remaining >= 3 {
		b.write(3, 2) // CL sym 17, code=11 reversed, 2 bits
		b.write(uint32(remaining-3), 3)
		remaining = 0
	}
	for remaining > 0 {
		// CL sym 0 not in our tree! Need to emit literal 0.
		// But CL sym 0 has code_length 0 in our CL tree... that's a problem!
		// We need CL sym 0 in our CL tree.
		fmt.Printf("WARNING: need CL sym 0 for %d remaining zeros (sym=%d)\n", remaining, sym)
		remaining--
	}

	// Emit the actual symbol's code_length (1)
	b.write(1, 2) // CL sym 1, code=01 reversed=10... wait

	// Actually, let me recalculate canonical codes for the CL tree.
	// Symbols with code_lengths: 1→2, 17→2, 18→1
	// Sorted by (length, symbol): (18,1), (1,2), (17,2)
	// blCount[0]=0, blCount[1]=1, blCount[2]=2
	// nextCode[1] = 0
	// nextCode[2] = (0+1)<<1 = 2
	// sym 18: raw code = nextCode[1] = 0, len 1 → reversed: 0 (1 bit)
	// sym 1:  raw code = nextCode[2] = 2 (binary 10), len 2 → reversed: 01 (2 bits)
	// sym 17: raw code = nextCode[2]+1 = 3 (binary 11), len 2 → reversed: 11 (2 bits)

	// So:
	// CL sym 18: write 0 (1 bit)
	// CL sym 1:  write 01 reversed = 01 → write value 1, 2 bits
	// CL sym 17: write 11 reversed = 11 → write value 3, 2 bits

	// Wait, I already emitted the code lengths. Let me redo this properly.
	fmt.Printf("  Normal tree: sym=%d, alphabetSize=%d\n", sym, alphabetSize)

	// Trailing zeros (after the symbol)
	remaining = alphabetSize - sym - 1
	for remaining >= 11 {
		n := remaining
		if n > 138 {
			n = 138
		}
		b.write(0, 1) // CL sym 18
		b.write(uint32(n-11), 7)
		remaining -= n
	}
	if remaining >= 3 {
		b.write(3, 2) // CL sym 17
		b.write(uint32(remaining-3), 3)
		remaining = 0
	}
	for remaining > 0 {
		fmt.Printf("WARNING: trailing zeros issue: %d remaining\n", remaining)
		remaining--
	}
}

func testPalette() {
	fmt.Println("\n=== Test 4: Palette 1x1 red (matching reference) ===")
	// Just use the reference VP8L data directly as a sanity check
	ref, _ := os.ReadFile("/tmp/red1x1_ref.webp")
	vp8lLen := binary.LittleEndian.Uint32(ref[16:20])
	fmt.Printf("Reference VP8L is %d bytes\n", vp8lLen)
	test("test_t4.webp", ref[20:20+vp8lLen])
}

func test(name string, vp8l []byte) {
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

	os.WriteFile(name, riff, 0644)
	fmt.Printf("VP8L data (%d bytes): ", len(vp8l))
	for _, v := range vp8l {
		fmt.Printf("%02x ", v)
	}
	fmt.Println()

	cmd := exec.Command("dwebp", name, "-o", "/tmp/"+name+".png")
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("FAIL: %s\n", string(out))
	} else {
		fmt.Println("OK!")
	}
}
