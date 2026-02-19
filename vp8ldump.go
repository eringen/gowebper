//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
)

type bitReader struct {
	data  []byte
	pos   int
	bits  uint64
	nBits int
	total int
}

func newBR(data []byte) *bitReader { return &bitReader{data: data} }

func (br *bitReader) fill() {
	for br.nBits <= 56 && br.pos < len(br.data) {
		br.bits |= uint64(br.data[br.pos]) << br.nBits
		br.nBits += 8
		br.pos++
	}
}

func (br *bitReader) read(n int) uint32 {
	br.fill()
	v := uint32(br.bits & ((1 << n) - 1))
	br.bits >>= n
	br.nBits -= n
	br.total += n
	return v
}

var codeLenOrder = [19]int{17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// Minimal Huffman tree for decoding
type huffTree struct {
	maxLen int
	table  []int16
}

func buildHuff(lengths []int) *huffTree {
	maxLen := 0
	for _, l := range lengths {
		if l > maxLen {
			maxLen = l
		}
	}
	if maxLen == 0 {
		maxLen = 1
	}
	blCount := make([]int, maxLen+1)
	for _, l := range lengths {
		if l > 0 {
			blCount[l]++
		}
	}
	nextCode := make([]uint32, maxLen+2)
	code := uint32(0)
	for bits := 1; bits <= maxLen; bits++ {
		code = (code + uint32(blCount[bits-1])) << 1
		nextCode[bits] = code
	}
	tableSize := 1 << maxLen
	table := make([]int16, tableSize)
	for i := range table {
		table[i] = -1
	}
	for sym, l := range lengths {
		if l == 0 {
			continue
		}
		raw := nextCode[l]
		nextCode[l]++
		rev := reverseBits(raw, l)
		step := 1 << l
		for i := int(rev); i < tableSize; i += step {
			table[i] = int16(sym)
		}
	}
	return &huffTree{maxLen: maxLen, table: table}
}

func reverseBits(v uint32, n int) uint32 {
	var r uint32
	for i := 0; i < n; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}

func (t *huffTree) readSym(br *bitReader) int {
	br.fill()
	peek := uint32(br.bits & ((1 << t.maxLen) - 1))
	sym := t.table[peek]
	if sym < 0 {
		fmt.Printf("    ERROR: invalid Huffman code 0x%x at bit %d\n", peek, br.total)
		return -1
	}
	// Find the actual code length for this symbol
	// We need lengths stored somewhere... simplified approach:
	// just consume maxLen bits (incorrect for variable length, but OK for dump)
	br.bits >>= t.maxLen
	br.nBits -= t.maxLen
	br.total += t.maxLen
	return int(sym)
}

func dumpVP8L(label string, data []byte) {
	fmt.Printf("\n=== %s (%d bytes) ===\n", label, len(data))
	br := newBR(data)

	sig := br.read(8)
	fmt.Printf("bit %3d: signature = 0x%02X\n", br.total, sig)
	w := br.read(14) + 1
	fmt.Printf("bit %3d: width = %d\n", br.total, w)
	h := br.read(14) + 1
	fmt.Printf("bit %3d: height = %d\n", br.total, h)
	alpha := br.read(1)
	fmt.Printf("bit %3d: alpha_is_used = %d\n", br.total, alpha)
	ver := br.read(3)
	fmt.Printf("bit %3d: version = %d\n", br.total, ver)

	for {
		more := br.read(1)
		fmt.Printf("bit %3d: more_transforms = %d\n", br.total, more)
		if more == 0 {
			break
		}
		kind := br.read(2)
		fmt.Printf("bit %3d: transform_type = %d\n", br.total, kind)
		switch kind {
		case 2:
			fmt.Println("  (SubtractGreen)")
		case 3:
			palSize := br.read(8) + 1
			fmt.Printf("bit %3d: palette_size = %d\n", br.total, palSize)
			fmt.Println("  --- palette sub-image ---")
			dumpHuffmanGroup(br, int(palSize), 1, "  ")
		case 0, 1:
			tileBits := br.read(3) + 2
			tilesW := (int(w) + (1<<tileBits) - 1) >> tileBits
			tilesH := (int(h) + (1<<tileBits) - 1) >> tileBits
			fmt.Printf("  tileBits=%d tiles=%dx%d\n", tileBits, tilesW, tilesH)
			dumpHuffmanGroup(br, tilesW, tilesH, "  ")
		}
	}
	fmt.Println("--- main image ---")
	dumpHuffmanGroup(br, int(w), int(h), "")
}

func dumpHuffmanGroup(br *bitReader, w, h int, indent string) {
	ccBits := 0
	if br.read(1) == 1 {
		ccBits = int(br.read(4))
	}
	fmt.Printf("%sbit %3d: color_cache_bits = %d\n", indent, br.total, ccBits)
	ccSize := 0
	if ccBits > 0 {
		ccSize = 1 << ccBits
	}
	names := []string{"G", "R", "B", "A", "D"}
	sizes := []int{256 + 24 + ccSize, 256, 256, 256, 40}
	for i, name := range names {
		dumpHuffTree(br, sizes[i], indent+name, indent)
	}
	fmt.Printf("%sbit %3d: pixel data starts (%dx%d = %d pixels)\n", indent, br.total, w, h, w*h)
}

func dumpHuffTree(br *bitReader, alphabetSize int, name, indent string) {
	startBit := br.total
	simple := br.read(1)
	if simple == 1 {
		numSym := br.read(1) + 1
		isFirst8 := br.read(1)
		sym1Bits := 1 + 7*int(isFirst8)
		sym1 := br.read(sym1Bits)
		if numSym == 1 {
			fmt.Printf("%s  %s: simple 1sym [is8=%d sym=%d] (%d bits @%d)\n",
				indent, name, isFirst8, sym1, br.total-startBit, startBit)
		} else {
			sym2 := br.read(8)
			fmt.Printf("%s  %s: simple 2sym [is8=%d s1=%d s2=%d] (%d bits @%d)\n",
				indent, name, isFirst8, sym1, sym2, br.total-startBit, startBit)
		}
		return
	}

	// Normal tree
	numCL := int(br.read(4)) + 4
	clLengths := make([]int, 19)
	for i := 0; i < numCL; i++ {
		clLengths[codeLenOrder[i]] = int(br.read(3))
	}

	// max_symbol flag
	maxSymbol := alphabetSize
	useLen := br.read(1)
	if useLen == 1 {
		lengthNbits := 2 + 2*int(br.read(3))
		maxSymbol = 2 + int(br.read(lengthNbits))
	}
	fmt.Printf("%s  %s: normal %dCL cl=%v useLen=%d maxSym=%d (%d bits @%d)\n",
		indent, name, numCL, clLengths, useLen, maxSymbol, br.total-startBit, startBit)

	// Build CL tree and decode code lengths
	clTree := buildHuff(clLengths)
	codeLens := make([]int, alphabetSize)
	i := 0
	prevLen := 0
	for i < maxSymbol {
		br.fill()
		peek := uint32(br.bits & ((1 << clTree.maxLen) - 1))
		sym := clTree.table[peek]
		if sym < 0 {
			fmt.Printf("%s    ERROR reading CL sym at i=%d bit=%d peek=0x%x\n", indent, i, br.total, peek)
			return
		}
		// Find actual bit length for this symbol
		symLen := 0
		for s := 0; s < 19; s++ {
			if clLengths[s] > 0 && int16(s) == sym {
				symLen = clLengths[s]
			}
		}
		if symLen == 0 {
			fmt.Printf("%s    ERROR: sym %d has zero CL length\n", indent, sym)
			return
		}
		br.bits >>= symLen
		br.nBits -= symLen
		br.total += symLen

		switch {
		case sym < 16:
			codeLens[i] = int(sym)
			prevLen = int(sym)
			i++
		case sym == 16:
			extra := int(br.read(2)) + 3
			for j := 0; j < extra && i < maxSymbol; j++ {
				codeLens[i] = prevLen
				i++
			}
		case sym == 17:
			extra := int(br.read(3)) + 3
			for j := 0; j < extra && i < maxSymbol; j++ {
				i++
			}
		case sym == 18:
			extra := int(br.read(7)) + 11
			for j := 0; j < extra && i < maxSymbol; j++ {
				i++
			}
		}
	}
	// Show non-zero code lengths
	nonZero := 0
	for j := 0; j < alphabetSize; j++ {
		if codeLens[j] > 0 {
			nonZero++
		}
	}
	fmt.Printf("%s    decoded %d code lengths, %d non-zero\n", indent, maxSymbol, nonZero)
	if nonZero <= 10 {
		for j := 0; j < alphabetSize; j++ {
			if codeLens[j] > 0 {
				fmt.Printf("%s      sym %d: len %d\n", indent, j, codeLens[j])
			}
		}
	}
}

func main() {
	for _, path := range os.Args[1:] {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatal(err)
		}
		if len(data) < 20 || string(data[0:4]) != "RIFF" || string(data[12:16]) != "VP8L" {
			log.Fatalf("%s: not a VP8L WebP", path)
		}
		chunkLen := binary.LittleEndian.Uint32(data[16:20])
		dumpVP8L(path, data[20:20+chunkLen])
	}
}
