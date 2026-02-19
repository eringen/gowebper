package huffman

import "github.com/eringen/gowebper/internal/bitwriter"

// VP8L code-length-of-code-length order (kCodeLengthCodeOrder).
var codeLenOrder = [19]int{17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// WriteTo serialises the Huffman tree t into a VP8L bitstream via bw.
// It automatically chooses between the simple (1 or 2 symbol) encoding
// and the normal code-length encoding.
func (t *Tree) WriteTo(bw *bitwriter.BitWriter) {
	// Count non-zero length symbols.
	nonZero := 0
	var firstSym, secondSym int
	for sym := 0; sym < t.AlphabetSize; sym++ {
		if t.Lengths[sym] > 0 {
			nonZero++
			if nonZero == 1 {
				firstSym = sym
			} else if nonZero == 2 {
				secondSym = sym
			}
		}
	}

	if nonZero <= 2 {
		writeSimpleTree(bw, t.AlphabetSize, nonZero, firstSym, secondSym)
		return
	}
	writeNormalTree(bw, t)
}

// writeSimpleTree writes the VP8L simple-code-length-code form.
// Used when the alphabet contains only 1 or 2 symbols.
//
// VP8L spec format:
//
//	simple_code_length_code  1 bit  (= 1)
//	num_symbols - 1          1 bit
//	is_first_8bits           1 bit  (0 → 1-bit symbol, 1 → 8-bit symbol)
//	first_symbol             1 or 8 bits
//	[second_symbol]          8 bits (only when num_symbols == 2)
func writeSimpleTree(bw *bitwriter.BitWriter, alphabetSize, numSymbols, sym1, sym2 int) {
	bw.WriteBits(1, 1)                    // simple_code_length_code = 1
	bw.WriteBits(uint32(numSymbols-1), 1) // num_symbols - 1

	if numSymbols == 2 && sym1 > sym2 {
		sym1, sym2 = sym2, sym1
	}

	// is_first_8bits: 0 if the first symbol fits in 1 bit (value 0 or 1),
	// 1 if it needs 8 bits.
	if sym1 > 1 {
		bw.WriteBits(1, 1)           // is_first_8bits = 1
		bw.WriteBits(uint32(sym1), 8) // first_symbol (8 bits)
	} else {
		bw.WriteBits(0, 1)           // is_first_8bits = 0
		bw.WriteBits(uint32(sym1), 1) // first_symbol (1 bit)
	}

	if numSymbols == 2 {
		bw.WriteBits(uint32(sym2), 8) // second_symbol (always 8 bits)
	}
}

// writeNormalTree writes the normal code-length-code form.
func writeNormalTree(bw *bitwriter.BitWriter, t *Tree) {
	bw.WriteBits(0, 1) // simple_code_lengths_code = 0

	// Build code-length sequence with RLE using symbols 16, 17, 18.
	clSeq := buildCodeLengthSeq(t.Lengths, t.AlphabetSize)

	// Build a second-level Huffman tree over the code-length alphabet (0..18).
	clBuilder := NewBuilder(19)
	for _, cl := range clSeq {
		clBuilder.Add(cl.sym)
	}
	clTree := clBuilder.BuildMaxLen(7) // VP8L CL code lengths are stored in 3 bits (0..7)

	// Determine how many code-length codes to emit (trim trailing zeros but
	// keep at least 4 as required by VP8L).
	numCLCodes := 19
	for numCLCodes > 4 && clTree.Lengths[codeLenOrder[numCLCodes-1]] == 0 {
		numCLCodes--
	}

	bw.WriteBits(uint32(numCLCodes-4), 4) // num_code_lengths - 4

	// Emit the code-length-of-code-length values in codeLenOrder.
	for i := 0; i < numCLCodes; i++ {
		bw.WriteBits(uint32(clTree.Lengths[codeLenOrder[i]]), 3)
	}

	// max_symbol flag: 0 means use the full alphabet size.
	bw.WriteBits(0, 1)

	// Emit the compressed code-length sequence.
	for _, cl := range clSeq {
		bw.WriteBits(clTree.Codes[cl.sym], clTree.Lengths[cl.sym])
		// Extra bits for RLE codes.
		switch cl.sym {
		case 16:
			bw.WriteBits(uint32(cl.extra), 2) // repeat count - 3
		case 17:
			bw.WriteBits(uint32(cl.extra), 3) // repeat count - 3
		case 18:
			bw.WriteBits(uint32(cl.extra), 7) // repeat count - 11
		}
	}
}

type clEntry struct {
	sym   int
	extra int // extra bits payload (already adjusted)
}

// buildCodeLengthSeq converts the code-length sequence to a run-length encoded
// sequence using the VP8L meta-symbols 16, 17, 18.
func buildCodeLengthSeq(lengths []int, size int) []clEntry {
	out := make([]clEntry, 0, size)
	i := 0
	for i < size {
		l := lengths[i]
		// Count run.
		j := i + 1
		for j < size && lengths[j] == l && j-i < 138 {
			j++
		}
		run := j - i

		if l == 0 {
			// Use symbol 17 (repeat 0, 3..10) or 18 (repeat 0, 11..138).
			for run > 0 {
				if run >= 11 {
					rep := run
					if rep > 138 {
						rep = 138
					}
					out = append(out, clEntry{18, rep - 11})
					run -= rep
				} else if run >= 3 {
					out = append(out, clEntry{17, run - 3})
					run = 0
				} else {
					out = append(out, clEntry{0, 0})
					run--
				}
			}
		} else {
			// Emit the first literal.
			out = append(out, clEntry{l, 0})
			run--
			// Remaining can use symbol 16 (repeat prev, 3..6).
			for run > 0 {
				if run >= 3 {
					rep := run
					if rep > 6 {
						rep = 6
					}
					out = append(out, clEntry{16, rep - 3})
					run -= rep
				} else {
					out = append(out, clEntry{l, 0})
					run--
				}
			}
		}
		i = j
	}
	return out
}
