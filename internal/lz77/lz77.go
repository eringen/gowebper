// Package lz77 implements LZ77 backward reference finding for VP8L.
// VP8L uses a 2D spatial distance table for the first 120 distance codes.
package lz77

// Token represents one element of the VP8L token stream.
// A token is either:
//   - A literal ARGB pixel (IsLiteral() == true)
//   - A backward reference (IsLiteral() == false && IsColorCache() == false)
//   - A color-cache reference (IsColorCache() == true)
type Token struct {
	kind     int    // tokenLiteral / tokenBackref / tokenColorCache
	pixel    uint32 // for literals
	length   int    // for back-refs: match length in pixels
	dist     int    // for back-refs: pixel distance (in pixel units)
	cacheIdx int    // for color-cache refs
}

const (
	tokenLiteral    = 0
	tokenBackref    = 1
	tokenColorCache = 2
)

// NewLiteral returns a literal token for the given ARGB pixel.
func NewLiteral(pixel uint32) Token {
	return Token{kind: tokenLiteral, pixel: pixel}
}

// NewBackref returns a backward-reference token.
func NewBackref(length, dist int) Token {
	return Token{kind: tokenBackref, length: length, dist: dist}
}

// NewColorCache returns a color-cache reference token.
func NewColorCache(idx int) Token {
	return Token{kind: tokenColorCache, cacheIdx: idx}
}

// IsLiteral reports whether t is a literal pixel token.
func (t Token) IsLiteral() bool { return t.kind == tokenLiteral }

// IsColorCache reports whether t is a color-cache reference.
func (t Token) IsColorCache() bool { return t.kind == tokenColorCache }

// Pixel returns the ARGB pixel for a literal token.
func (t Token) Pixel() uint32 { return t.pixel }

// CacheIndex returns the cache slot index for a color-cache token.
func (t Token) CacheIndex() int { return t.cacheIdx }

// LengthCode returns the VP8L length code (0..23) for a back-reference token.
func (t Token) LengthCode() int { return lengthCode(t.length) }

// LengthExtra returns the extra bits value for the length code.
func (t Token) LengthExtra() int { return lengthExtra(t.length) }

// LengthExtraBits returns how many extra bits the length code carries.
func LengthExtraBits(code int) int { return lengthExtraBits(code) }

// DistCode returns the VP8L distance code (0..39) for a back-reference token.
// t.dist holds the pre-computed VP8L coded distance (set at tokenize time).
func (t Token) DistCode() int {
	dc, _, _ := distCodeOf(t.dist)
	return dc
}

// DistExtra returns the extra bits value for the distance code.
func (t Token) DistExtra() int {
	_, de, _ := distCodeOf(t.dist)
	return de
}

// DistExtraBits returns how many extra bits the distance code carries.
func (t Token) DistExtraBits() int {
	_, _, bits := distCodeOf(t.dist)
	return bits
}

// ---- Length / distance coding (VP8L spec §7.2.3) ----

// lengthCode maps a match length (>= 1) to a VP8L length code in [0, 23].
func lengthCode(length int) int {
	l := length - 1 // VP8L encodes length-1
	if l < 4 {
		return l
	}
	// extra_bits = floor(log2(l)) - 1 = msb32(l) - 2
	n := msb32(uint32(l)) - 2
	return 2*n + 2 + int((uint32(l)>>n)&1)
}

// lengthExtra returns the extra bits value for a given length.
func lengthExtra(length int) int {
	l := length - 1
	if l < 4 {
		return 0
	}
	code := lengthCode(length)
	offset := lengthOffset(code)
	return l - offset
}

// lengthExtraBits returns the number of extra bits for a length code.
func lengthExtraBits(code int) int {
	if code < 4 {
		return 0
	}
	return (code - 2) >> 1
}

// lengthOffset returns the base length (before extra bits) for a length code.
func lengthOffset(code int) int {
	if code < 4 {
		return code
	}
	extraBits := (code - 2) >> 1
	return ((2 + (code & 1)) << extraBits)
}

// distCodeOf returns (code, extra, extraBits) for a VP8L coded distance value.
// The VP8L distance value is >= 1.
func distCodeOf(vDist int) (code, extra, extraBits int) {
	if vDist < 5 {
		return vDist - 1, 0, 0
	}
	l := vDist - 1
	n := msb32(uint32(l)) - 2 // extra_bits = floor(log2(l)) - 1
	code = 2*n + 2 + int((uint32(l)>>n)&1)
	extraBits = n
	offset := (2 + (code & 1)) << n
	extra = l - offset
	return
}

// msb32 returns floor(log2(v)) + 1 for v > 0.
func msb32(v uint32) int {
	n := 0
	for v > 0 {
		v >>= 1
		n++
	}
	return n
}

// ---- VP8L spatial distance table ----
// Maps VP8L "pixel distance code" to (dx, dy).
// The first 120 entries have explicit spatial offsets; beyond 120 the pixel
// distance equals code - 120.

var distOffsets = [120][2]int{
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

// buildSpatialTable builds a reverse lookup from linear pixel distance to the
// optimal VP8L raw distance code (5..120) for the given image width.
//
// Background: VP8L distance Huffman codes 0..3 decode as pixel distances 1..4
// directly (the decoder returns code+1 without consulting the spatial table).
// Therefore only spatial entries 4..119 (VP8L raw distances 5..120) can be
// used through the spatial path. For pixel distances 1..4, linearToVP8LDist
// uses raw distances 1..4 directly (→ codes 0..3).
func buildSpatialTable(width int) map[int]int {
	table := make(map[int]int, 116)
	for k := 4; k < len(distOffsets); k++ {
		off := distOffsets[k]
		// VP8L pixel distance = dy*width + dx (signed, matching the decoder).
		pd := off[1]*width + off[0]
		if pd < 1 {
			pd = 1
		}
		// Keep only the smallest (first) VP8L code for each pixel distance.
		if _, exists := table[pd]; !exists {
			table[pd] = k + 1 // 1-indexed VP8L raw distance (5..120)
		}
	}
	return table
}

// linearToVP8LDist converts a linear pixel array distance to the best VP8L
// raw distance value:
//   - Pixel dists 1..4 map directly to VP8L raw dists 1..4 (Huffman codes 0..3),
//     which the decoder returns as pixel distances 1..4 without spatial lookup.
//   - Larger pixel dists use the pre-built spatial table when possible, falling
//     back to the generic form (pixDist+120) otherwise.
func linearToVP8LDist(pixDist int, spatialTable map[int]int) int {
	if pixDist <= 4 {
		return pixDist // VP8L raw dist 1..4 → Huffman codes 0..3
	}
	if code, ok := spatialTable[pixDist]; ok {
		return code
	}
	return pixDist + 120
}

// Tokenise converts the flat pixel array to a VP8L token stream using LZ77
// hash-chain matching with the given window and chain-depth parameters.
// ccBits > 0 enables color cache references.
// Token.dist for backward references stores the pre-computed VP8L coded
// distance (using the spatial table for the given width where possible).
func Tokenise(pixels []uint32, width, lz77Window, chainDepth, ccBits int) []Token {
	n := len(pixels)
	if n == 0 || lz77Window == 0 {
		return literalTokens(pixels, ccBits)
	}

	// Pre-build reverse spatial table for this image width.
	spatialTable := buildSpatialTable(width)

	// Color cache.
	var colorCache []uint32
	ccMask := 0
	if ccBits > 0 {
		colorCache = make([]uint32, 1<<ccBits)
		ccMask = (1 << ccBits) - 1
	}

	// Hash chain: hashHead[h] = most recent position with that hash.
	hashSize := lz77Window
	if hashSize > n {
		hashSize = n
	}
	hashHead := make([]int, 1<<17) // 2^17 buckets
	for i := range hashHead {
		hashHead[i] = -1
	}
	prev := make([]int, n)
	for i := range prev {
		prev[i] = -1
	}

	tokens := make([]Token, 0, n)
	i := 0
	for i < n {
		// Try color cache first.
		if ccBits > 0 {
			h := int((pixels[i] * 0x1e35a7bd) >> (32 - ccBits))
			if colorCache[h&ccMask] == pixels[i] {
				tokens = append(tokens, NewColorCache(h&ccMask))
				colorCache[h&ccMask] = pixels[i]
				updateHash(hashHead, prev, pixels, i, width)
				i++
				continue
			}
		}

		// Try LZ77 backward match.
		if i >= 1 {
			bestLen, bestDist := findBestMatch(pixels, i, n, width, lz77Window, chainDepth, hashHead, prev)
			if bestLen >= 1 {
				vp8lDist := linearToVP8LDist(bestDist, spatialTable)
				tokens = append(tokens, NewBackref(bestLen, vp8lDist))
				// Advance i and update hash for each consumed pixel.
				for k := 0; k < bestLen && i < n; k++ {
					if ccBits > 0 {
						h := int((pixels[i] * 0x1e35a7bd) >> (32 - ccBits))
						colorCache[h&ccMask] = pixels[i]
					}
					updateHash(hashHead, prev, pixels, i, width)
					i++
				}
				continue
			}
		}

		// Literal.
		if ccBits > 0 {
			h := int((pixels[i] * 0x1e35a7bd) >> (32 - ccBits))
			colorCache[h&ccMask] = pixels[i]
		}
		updateHash(hashHead, prev, pixels, i, width)
		tokens = append(tokens, NewLiteral(pixels[i]))
		i++
	}
	return tokens
}

func literalTokens(pixels []uint32, ccBits int) []Token {
	tokens := make([]Token, len(pixels))
	for i, p := range pixels {
		tokens[i] = NewLiteral(p)
	}
	return tokens
}

// pixelHash computes a hash for a pixel value suitable for the hash chain.
func pixelHash(p uint32) int {
	return int((p * 0x1e35a7bd) >> 15 & 0x1FFFF)
}

func updateHash(hashHead, prev []int, pixels []uint32, i, width int) {
	if i >= len(pixels) {
		return
	}
	h := pixelHash(pixels[i])
	prev[i] = hashHead[h]
	hashHead[h] = i
}

// findBestMatch searches for the best backward match at position i.
// Returns (length, pixDist) where pixDist >= 1.
func findBestMatch(pixels []uint32, i, n, width, window, chainDepth int, hashHead, prev []int) (int, int) {
	if i == 0 {
		return 0, 0
	}

	h := pixelHash(pixels[i])
	best := 0
	bestDist := 0
	minPos := i - window
	if minPos < 0 {
		minPos = 0
	}

	j := hashHead[h]
	checked := 0
	for j >= minPos && j >= 0 && checked < chainDepth {
		if pixels[j] == pixels[i] {
			// Measure match length.
			l := matchLen(pixels, i, j, n)
			if l > best {
				best = l
				bestDist = i - j
			}
		}
		j = prev[j]
		checked++
	}
	if best < 1 {
		return 0, 0
	}
	return best, bestDist
}

// matchLen returns the number of consecutive matching pixels starting at a and b.
func matchLen(pixels []uint32, a, b, n int) int {
	maxLen := n - a
	if a-b < maxLen {
		maxLen = a - b
	}
	// VP8L max match length is 2^24 - 1 but practically limited.
	// Limit to a reasonable value for performance.
	const maxMatch = 4096
	if maxLen > maxMatch {
		maxLen = maxMatch
	}
	l := 0
	for l < maxLen && pixels[a+l] == pixels[b+l] {
		l++
	}
	return l
}
