//go:build ignore

package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/exec"
)

func main() {
	// Take VP8L data from reference file and re-wrap in our RIFF container.
	refData, err := os.ReadFile("/tmp/red1x1_ref.webp")
	if err != nil {
		log.Fatal(err)
	}
	vp8lLen := binary.LittleEndian.Uint32(refData[16:20])
	vp8lData := refData[20 : 20+vp8lLen]
	fmt.Printf("Reference VP8L data (%d bytes): ", vp8lLen)
	for _, b := range vp8lData {
		fmt.Printf("%02x ", b)
	}
	fmt.Println()

	// Wrap in our RIFF container
	padded := len(vp8lData)
	if padded%2 != 0 {
		padded++
	}
	totalSize := 4 + 4 + 4 + padded
	riff := make([]byte, 8+totalSize)
	copy(riff[0:4], "RIFF")
	binary.LittleEndian.PutUint32(riff[4:8], uint32(totalSize))
	copy(riff[8:12], "WEBP")
	copy(riff[12:16], "VP8L")
	binary.LittleEndian.PutUint32(riff[16:20], uint32(len(vp8lData)))
	copy(riff[20:], vp8lData)

	os.WriteFile("test_rewrap.webp", riff, 0644)
	fmt.Printf("Rewrapped: %d bytes\n", len(riff))

	// Test
	cmd := exec.Command("dwebp", "test_rewrap.webp", "-o", "/tmp/rewrap.png")
	out, _ := cmd.CombinedOutput()
	fmt.Println(string(out))

	// Now try creating a minimal file with just no-transform, no-cache,
	// and all trees being normal (empty) trees instead of simple trees.
	// This matches the reference format more closely.
	fmt.Println("\n=== Test: all-normal-tree format ===")

	// Reference main image uses normal trees with all code lengths = 0
	// for a "dead" tree. Let's build this manually.
	//
	// Normal tree format:
	// simple_code = 0
	// num_code_lengths = ReadBits(4) + 4 → min 4 CL values
	// For each CL: ReadBits(3) → 0 for empty
	// max_symbol flag: ReadBits(1) → 0 (use full alphabet)
	// Then 0 CL symbols decoded (all empty)
	//
	// For an empty CL tree (all lengths 0), no code lengths can be decoded,
	// so 0 symbols are decoded. The tree is completely empty.

	// Let's just manually compare the bits from position 88 onward
	// between our file and the reference.
	ourData, _ := os.ReadFile("test_manual.webp")
	fmt.Printf("\nOur VP8L data (%d bytes): ", len(ourData)-20)
	for _, b := range ourData[20:] {
		fmt.Printf("%02x ", b)
	}
	fmt.Println()
	fmt.Printf("Ref VP8L data (%d bytes): ", len(vp8lData))
	for _, b := range vp8lData {
		fmt.Printf("%02x ", b)
	}
	fmt.Println()

	// Decode bit by bit from position 42 (start of Huffman group)
	fmt.Println("\nBit comparison from position 42:")
	ourBits := extractBits(ourData[20:], 42, 40)
	refBits := extractBits(vp8lData, 42, 40)
	fmt.Printf("Our: %s\n", ourBits)
	fmt.Printf("Ref: %s\n", refBits)
}

func extractBits(data []byte, startBit, count int) string {
	result := ""
	for i := 0; i < count; i++ {
		bit := startBit + i
		byteIdx := bit / 8
		bitIdx := bit % 8
		if byteIdx >= len(data) {
			result += "?"
			continue
		}
		if data[byteIdx]&(1<<bitIdx) != 0 {
			result += "1"
		} else {
			result += "0"
		}
		if (i+1)%4 == 0 {
			result += " "
		}
	}
	return result
}
