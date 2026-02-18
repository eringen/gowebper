package huffman

import (
	"bytes"
	"testing"

	"github.com/eringen/gowebper/internal/bitwriter"
)

func TestBuilder_singleSymbol(t *testing.T) {
	b := NewBuilder(256)
	b.Add(42)
	tree := b.Build()

	if tree.Lengths[42] != 1 {
		t.Errorf("single symbol should have length 1, got %d", tree.Lengths[42])
	}
	for i := 0; i < 256; i++ {
		if i != 42 && tree.Lengths[i] != 0 {
			t.Errorf("symbol %d should have length 0, got %d", i, tree.Lengths[i])
		}
	}
}

func TestBuilder_twoSymbols(t *testing.T) {
	b := NewBuilder(256)
	b.Add(0)
	b.Add(255)
	tree := b.Build()

	if tree.Lengths[0] != 1 {
		t.Errorf("symbol 0 should have length 1, got %d", tree.Lengths[0])
	}
	if tree.Lengths[255] != 1 {
		t.Errorf("symbol 255 should have length 1, got %d", tree.Lengths[255])
	}
}

func TestBuilder_frequencyOrderedLengths(t *testing.T) {
	// Higher-frequency symbols should get shorter or equal code lengths.
	b := NewBuilder(4)
	b.AddN(0, 100)
	b.AddN(1, 50)
	b.AddN(2, 25)
	b.AddN(3, 1)
	tree := b.Build()

	if tree.Lengths[0] > tree.Lengths[3] {
		t.Errorf("most frequent symbol should have shorter length: len[0]=%d len[3]=%d",
			tree.Lengths[0], tree.Lengths[3])
	}
	if tree.Lengths[1] > tree.Lengths[3] {
		t.Errorf("symbol 1 should have length <= symbol 3: %d vs %d",
			tree.Lengths[1], tree.Lengths[3])
	}
}

func TestBuilder_maxCodeLength(t *testing.T) {
	// With 256 symbols all with frequency 1, no code should exceed 15 bits.
	b := NewBuilder(256)
	for i := 0; i < 256; i++ {
		b.Add(i)
	}
	tree := b.Build()

	for i := 0; i < 256; i++ {
		if tree.Lengths[i] > maxCodeLen {
			t.Errorf("symbol %d has length %d exceeding max %d", i, tree.Lengths[i], maxCodeLen)
		}
	}
}

func TestBuilder_canonicalCodes(t *testing.T) {
	// Verify codes are canonical: within the same length, codes are consecutive.
	b := NewBuilder(8)
	for i := 0; i < 8; i++ {
		b.Add(i)
	}
	tree := b.Build()

	// Group by length.
	byLen := make(map[int][]int)
	for sym := 0; sym < 8; sym++ {
		l := tree.Lengths[sym]
		if l > 0 {
			byLen[l] = append(byLen[l], sym)
		}
	}
	// For each length, the bit-reversed codes should differ by 1 each step.
	for _, syms := range byLen {
		if len(syms) < 2 {
			continue
		}
		for i := 1; i < len(syms); i++ {
			diff := int(reverseBits(tree.Codes[syms[i]], tree.Lengths[syms[i]])) -
				int(reverseBits(tree.Codes[syms[i-1]], tree.Lengths[syms[i-1]]))
			if diff != 1 {
				t.Errorf("non-canonical codes at length %d: diff %d", tree.Lengths[syms[i]], diff)
			}
		}
	}
}

func TestBuildFromLengths(t *testing.T) {
	lengths := []int{2, 1, 3, 3}
	tree := BuildFromLengths(lengths)

	for i, l := range lengths {
		if tree.Lengths[i] != l {
			t.Errorf("symbol %d: got length %d, want %d", i, tree.Lengths[i], l)
		}
	}
}

func TestBitCost(t *testing.T) {
	b := NewBuilder(4)
	b.AddN(0, 8)
	b.AddN(1, 4)
	b.AddN(2, 2)
	b.AddN(3, 1)
	tree := b.Build()

	freq := []uint32{8, 4, 2, 1}
	cost := tree.BitCost(freq)
	if cost == 0 {
		t.Error("expected non-zero bit cost")
	}
	// Optimal Huffman for these frequencies uses ~22 bits (8*1 + 4*2 + 2*3 + 1*3 = 27 or so)
	// Just verify it's a reasonable value.
	if cost > 100 {
		t.Errorf("bit cost %d seems too high", cost)
	}
}

func TestWriteTo_simpleOneSymbol(t *testing.T) {
	b := NewBuilder(256)
	b.Add(0)
	tree := b.Build()

	var buf bytes.Buffer
	bw := bitwriter.New(&buf)
	tree.WriteTo(bw)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	// simple form: first bit = 1, then num_symbols-1 = 0, then symbol value.
	// We just check the buffer is non-empty and starts correctly.
	data := buf.Bytes()
	if len(data) == 0 {
		t.Error("expected non-empty output for simple tree")
	}
	// First bit should be 1 (simple_code_lengths_code).
	if data[0]&1 != 1 {
		t.Errorf("expected simple code form (bit0=1), got byte 0x%02X", data[0])
	}
}

func TestWriteTo_normalTree(t *testing.T) {
	// Build a tree with many symbols so normal form is chosen.
	b := NewBuilder(256)
	for i := 0; i < 256; i++ {
		b.AddN(i, 1+i)
	}
	tree := b.Build()

	var buf bytes.Buffer
	bw := bitwriter.New(&buf)
	tree.WriteTo(bw)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	if len(data) == 0 {
		t.Error("expected non-empty output for normal tree")
	}
	// First bit = 0 (normal code form).
	if data[0]&1 != 0 {
		t.Errorf("expected normal code form (bit0=0), got byte 0x%02X", data[0])
	}
}

func TestBuildCodeLengthSeq_basic(t *testing.T) {
	// A sequence of lengths with some zeros should use RLE symbols.
	lengths := make([]int, 20)
	lengths[0] = 3
	lengths[1] = 3
	lengths[2] = 3
	// lengths[3..19] = 0

	seq := buildCodeLengthSeq(lengths, 20)
	if len(seq) == 0 {
		t.Error("expected non-empty code-length sequence")
	}
	// Verify that the first entry is a literal (length 3).
	if seq[0].sym != 3 {
		t.Errorf("expected first entry sym=3, got %d", seq[0].sym)
	}
}
