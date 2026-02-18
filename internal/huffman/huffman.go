// Package huffman implements canonical Huffman code construction for VP8L.
// Codes are length-limited to 15 bits as required by the VP8L specification.
package huffman

import (
	"container/heap"
	"sort"
)

const maxCodeLen = 15

// Tree holds a complete canonical Huffman code table.
type Tree struct {
	// Codes and Lengths are indexed by symbol.
	Codes   []uint32
	Lengths []int
	// AlphabetSize is the number of symbols including trailing zeros.
	AlphabetSize int
}

// Builder accumulates symbol frequencies and builds a Huffman tree.
type Builder struct {
	freq []uint32
	size int
}

// NewBuilder returns a Builder for an alphabet of size n.
func NewBuilder(n int) *Builder {
	return &Builder{freq: make([]uint32, n), size: n}
}

// Add records one occurrence of symbol sym.
func (b *Builder) Add(sym int) {
	if sym >= 0 && sym < b.size {
		b.freq[sym]++
	}
}

// AddN records n occurrences of symbol sym.
func (b *Builder) AddN(sym, n int) {
	if sym >= 0 && sym < b.size && n > 0 {
		b.freq[sym] += uint32(n)
	}
}

// Build constructs a length-limited canonical Huffman tree (max 15 bits).
func (b *Builder) Build() *Tree {
	return buildTree(b.freq, b.size)
}

// BuildFromLengths constructs a canonical Huffman tree from explicit code lengths.
func BuildFromLengths(lengths []int) *Tree {
	t := &Tree{
		Codes:        make([]uint32, len(lengths)),
		Lengths:      make([]int, len(lengths)),
		AlphabetSize: len(lengths),
	}
	copy(t.Lengths, lengths)
	assignCanonicalCodes(t)
	return t
}

// BitCost returns the total bit cost to encode all symbols given their frequencies.
func (t *Tree) BitCost(freq []uint32) uint64 {
	var cost uint64
	for i, f := range freq {
		if i < t.AlphabetSize {
			cost += uint64(f) * uint64(t.Lengths[i])
		}
	}
	return cost
}

// ---- internal ----

// huffNode is a node in the Huffman tree during construction.
type huffNode struct {
	freq  uint32
	sym   int // -1 for internal nodes
	left  *huffNode
	right *huffNode
}

// huffHeap implements heap.Interface for *huffNode (min-heap by frequency).
type huffHeap []*huffNode

func (h huffHeap) Len() int { return len(h) }
func (h huffHeap) Less(i, j int) bool {
	if h[i].freq != h[j].freq {
		return h[i].freq < h[j].freq
	}
	// Deterministic tie-breaking: prefer leaves (sym >= 0) over internal nodes,
	// then prefer lower symbol index.
	li, lj := h[i].sym >= 0, h[j].sym >= 0
	if li != lj {
		return li // leaves first
	}
	if li {
		return h[i].sym < h[j].sym
	}
	return false
}
func (h huffHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *huffHeap) Push(x interface{}) {
	*h = append(*h, x.(*huffNode))
}
func (h *huffHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

func buildTree(freq []uint32, size int) *Tree {
	t := &Tree{
		Codes:        make([]uint32, size),
		Lengths:      make([]int, size),
		AlphabetSize: size,
	}

	// Count active symbols.
	active := 0
	for i := 0; i < size; i++ {
		if freq[i] > 0 {
			active++
		}
	}

	if active == 0 {
		return t
	}
	if active == 1 {
		for i := 0; i < size; i++ {
			if freq[i] > 0 {
				t.Lengths[i] = 1
				break
			}
		}
		assignCanonicalCodes(t)
		return t
	}

	// Build a standard Huffman tree using a min-heap.
	h := &huffHeap{}
	heap.Init(h)
	for i := 0; i < size; i++ {
		if freq[i] > 0 {
			heap.Push(h, &huffNode{freq: freq[i], sym: i})
		}
	}
	for h.Len() > 1 {
		a := heap.Pop(h).(*huffNode)
		b := heap.Pop(h).(*huffNode)
		heap.Push(h, &huffNode{
			freq:  a.freq + b.freq,
			sym:   -1,
			left:  a,
			right: b,
		})
	}

	// Extract code lengths by DFS.
	root := heap.Pop(h).(*huffNode)
	extractLengths(root, 0, t.Lengths)

	// Limit lengths to maxCodeLen.
	limitLengths(t.Lengths, size, maxCodeLen)

	assignCanonicalCodes(t)
	return t
}

// extractLengths fills lengths[sym] = depth for each leaf node.
func extractLengths(n *huffNode, depth int, lengths []int) {
	if n.sym >= 0 {
		lengths[n.sym] = depth
		return
	}
	if n.left != nil {
		extractLengths(n.left, depth+1, lengths)
	}
	if n.right != nil {
		extractLengths(n.right, depth+1, lengths)
	}
}

// limitLengths adjusts lengths so none exceeds maxLen, maintaining Kraft validity.
func limitLengths(lengths []int, size, maxLen int) {
	// Clamp any depths exceeding maxLen.
	clamped := false
	for i := 0; i < size; i++ {
		if lengths[i] > maxLen {
			lengths[i] = maxLen
			clamped = true
		}
	}
	if !clamped {
		return
	}

	// Compute Kraft excess in units of 2^maxLen:
	// excess = sum(2^(maxLen - l_i)) - 2^maxLen
	// Positive excess means Kraft inequality is violated; we must lengthen codes.
	excess := -(int64(1) << maxLen)
	for _, l := range lengths {
		if l > 0 {
			excess += int64(1) << (maxLen - l)
		}
	}
	if excess <= 0 {
		return
	}

	// Sort symbols by length descending (longest first = rarest = best to lengthen).
	type entry struct{ idx, depth int }
	entries := make([]entry, 0, size)
	for i, l := range lengths {
		if l > 0 && l < maxLen {
			entries = append(entries, entry{i, l})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].depth > entries[j].depth
	})

	// Lengthen codes until excess is resolved.
	for excess > 0 {
		changed := false
		for i := range entries {
			if entries[i].depth < maxLen && excess > 0 {
				entries[i].depth++
				lengths[entries[i].idx] = entries[i].depth
				// Lengthening reduces excess by 2^(maxLen - newDepth).
				excess -= int64(1) << (maxLen - entries[i].depth)
				changed = true
			}
		}
		if !changed {
			break
		}
	}
}

// assignCanonicalCodes fills t.Codes from t.Lengths using the standard
// canonical Huffman code assignment (sorted by symbol index).
func assignCanonicalCodes(t *Tree) {
	// Count codes per length.
	var blCount [maxCodeLen + 1]int
	for _, l := range t.Lengths {
		if l > 0 {
			blCount[l]++
		}
	}

	// Find the start code for each length.
	var nextCode [maxCodeLen + 2]uint32
	code := uint32(0)
	blCount[0] = 0
	for bits := 1; bits <= maxCodeLen; bits++ {
		code = (code + uint32(blCount[bits-1])) << 1
		nextCode[bits] = code
	}

	// Assign codes in symbol order.
	for sym := 0; sym < t.AlphabetSize; sym++ {
		if l := t.Lengths[sym]; l > 0 {
			raw := nextCode[l]
			nextCode[l]++
			t.Codes[sym] = reverseBits(raw, l)
		}
	}
}

// reverseBits reverses the low n bits of v.
func reverseBits(v uint32, n int) uint32 {
	var r uint32
	for i := 0; i < n; i++ {
		r = (r << 1) | (v & 1)
		v >>= 1
	}
	return r
}
