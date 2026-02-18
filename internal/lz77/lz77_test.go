package lz77

import (
	"testing"
)

func TestNewLiteral(t *testing.T) {
	tok := NewLiteral(0xFFFF0000)
	if !tok.IsLiteral() {
		t.Error("expected IsLiteral() = true")
	}
	if tok.Pixel() != 0xFFFF0000 {
		t.Errorf("pixel: got 0x%08X, want 0xFFFF0000", tok.Pixel())
	}
}

func TestNewBackref(t *testing.T) {
	tok := NewBackref(5, 3)
	if tok.IsLiteral() {
		t.Error("expected IsLiteral() = false")
	}
	if tok.IsColorCache() {
		t.Error("expected IsColorCache() = false")
	}
	if tok.length != 5 || tok.dist != 3 {
		t.Errorf("got length=%d dist=%d, want 5 3", tok.length, tok.dist)
	}
}

func TestNewColorCache(t *testing.T) {
	tok := NewColorCache(7)
	if !tok.IsColorCache() {
		t.Error("expected IsColorCache() = true")
	}
	if tok.CacheIndex() != 7 {
		t.Errorf("cache index: got %d, want 7", tok.CacheIndex())
	}
}

func TestLengthCode(t *testing.T) {
	// VP8L length codes for small lengths.
	cases := []struct {
		length int
		code   int
	}{
		{1, 0},
		{2, 1},
		{3, 2},
		{4, 3},
		{5, 4},
		{6, 4},
		{7, 5},
		{8, 5},
	}
	for _, tc := range cases {
		got := lengthCode(tc.length)
		if got != tc.code {
			t.Errorf("lengthCode(%d) = %d, want %d", tc.length, got, tc.code)
		}
	}
}

func TestLengthExtraBits(t *testing.T) {
	// Code 0..3: 0 extra bits.
	for code := 0; code < 4; code++ {
		if got := LengthExtraBits(code); got != 0 {
			t.Errorf("LengthExtraBits(%d) = %d, want 0", code, got)
		}
	}
	// Code 4 and 5: 1 extra bit.
	for _, code := range []int{4, 5} {
		if got := LengthExtraBits(code); got != 1 {
			t.Errorf("LengthExtraBits(%d) = %d, want 1", code, got)
		}
	}
	// Code 6 and 7: 2 extra bits.
	for _, code := range []int{6, 7} {
		if got := LengthExtraBits(code); got != 2 {
			t.Errorf("LengthExtraBits(%d) = %d, want 2", code, got)
		}
	}
}

func TestTokenise_allLiterals(t *testing.T) {
	// Unique pixels: no repetition, so all should be literals.
	pixels := []uint32{0xFF000001, 0xFF000002, 0xFF000003, 0xFF000004}
	tokens := Tokenise(pixels, 4, 1024, 4, 0)

	if len(tokens) != 4 {
		t.Fatalf("expected 4 tokens, got %d", len(tokens))
	}
	for i, tok := range tokens {
		if !tok.IsLiteral() {
			t.Errorf("token %d: expected literal, got backref", i)
		}
		if tok.Pixel() != pixels[i] {
			t.Errorf("token %d: pixel 0x%08X, want 0x%08X", i, tok.Pixel(), pixels[i])
		}
	}
}

func TestTokenise_withRepeat(t *testing.T) {
	// Repeated pixels should produce at least one backward reference.
	p := uint32(0xFFABCDEF)
	pixels := make([]uint32, 20)
	// First 5 are unique, then 15 repeat the first pixel.
	for i := 0; i < 5; i++ {
		pixels[i] = uint32(0xFF000000 | i)
	}
	for i := 5; i < 20; i++ {
		pixels[i] = p
	}
	_ = p

	tokens := Tokenise(pixels, 10, 1024, 4, 0)

	// Must decode back to same pixels.
	decoded := decodeTokens(tokens)
	if len(decoded) != len(pixels) {
		t.Fatalf("decoded %d pixels, want %d", len(decoded), len(pixels))
	}
	for i := range pixels {
		if decoded[i] != pixels[i] {
			t.Errorf("decoded pixel %d: got 0x%08X, want 0x%08X", i, decoded[i], pixels[i])
		}
	}

	// There should be at least one backref token for the repeated sequence.
	hasBackref := false
	for _, tok := range tokens {
		if !tok.IsLiteral() && !tok.IsColorCache() {
			hasBackref = true
			break
		}
	}
	if !hasBackref {
		t.Error("expected at least one backward reference token for repeated sequence")
	}
}

func TestTokenise_empty(t *testing.T) {
	tokens := Tokenise(nil, 0, 1024, 4, 0)
	if len(tokens) != 0 {
		t.Errorf("expected 0 tokens for empty input, got %d", len(tokens))
	}
}

func TestDistCodeOf(t *testing.T) {
	// Small distances should have code < 4.
	code, _, _ := distCodeOf(1)
	if code != 0 {
		t.Errorf("distCodeOf(1) code=%d, want 0", code)
	}
	code, _, _ = distCodeOf(4)
	if code != 3 {
		t.Errorf("distCodeOf(4) code=%d, want 3", code)
	}
}

// decodeTokens simulates VP8L decoding of tokens to recover pixels.
func decodeTokens(tokens []Token) []uint32 {
	var out []uint32
	for _, tok := range tokens {
		if tok.IsLiteral() {
			out = append(out, tok.Pixel())
		} else if !tok.IsColorCache() {
			src := len(out) - tok.dist
			if src < 0 {
				src = 0
			}
			for k := 0; k < tok.length; k++ {
				out = append(out, out[src+k])
			}
		}
	}
	return out
}
