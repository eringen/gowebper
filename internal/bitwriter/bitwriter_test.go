package bitwriter

import (
	"bytes"
	"testing"
)

func TestWriteBits_singleByte(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	// Write 0b10110011 = 0xB3 LSB first (just 8 bits of the value 0xB3).
	bw.WriteBits(0xB3, 8)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if got := buf.Bytes(); len(got) != 1 || got[0] != 0xB3 {
		t.Errorf("got %v, want [0xB3]", got)
	}
}

func TestWriteBits_crossByteBoundary(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	// Write 4 bits then 8 bits.
	// bits 0..3: 0b1010 (0xA) -> low nibble of byte 0
	// bits 4..11: 0b11001100 (0xCC) -> high nibble of byte 0 | byte 1
	bw.WriteBits(0xA, 4)  // bits  0-3
	bw.WriteBits(0xCC, 8) // bits 4-11
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	// byte0 = low 8 bits: 0xCA (low4=A, next4=low4 of CC=C -> 0xCA)
	// byte1 = remaining 4 bits of CC = 0xC -> padded to 0x0C
	got := buf.Bytes()
	if len(got) != 2 {
		t.Fatalf("expected 2 bytes, got %d: %v", len(got), got)
	}
	if got[0] != 0xCA {
		t.Errorf("byte0: got 0x%02X, want 0xCA", got[0])
	}
	if got[1] != 0x0C {
		t.Errorf("byte1: got 0x%02X, want 0x0C", got[1])
	}
}

func TestWriteBit(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	// Write bits: 1 0 1 1 0 0 0 1 = 0b10001101 = 0x8D (LSB first means
	// first written bit is bit 0, so byte = 1|0<<1|1<<2|1<<3|0<<4|0<<5|0<<6|1<<7)
	// = 0x8D
	bits := []int{1, 0, 1, 1, 0, 0, 0, 1}
	for _, b := range bits {
		bw.WriteBit(b)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	if len(got) != 1 || got[0] != 0x8D {
		t.Errorf("got %v, want [0x8D]", got)
	}
}

func TestWriteBits_zero(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	bw.WriteBits(0xFF, 0) // write 0 bits, should have no effect
	bw.WriteBits(0x01, 1)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	if len(got) != 1 || got[0] != 0x01 {
		t.Errorf("got %v, want [0x01]", got)
	}
}

func TestWriteBits_32bits(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	bw.WriteBits(0xDEADBEEF, 32)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	want := []byte{0xEF, 0xBE, 0xAD, 0xDE} // little-endian
	if !bytes.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWriteBits_knownVP8LHeader(t *testing.T) {
	// VP8L header for a 1x1 image:
	// magic=0x2F (8 bits), width-1=0 (14 bits), height-1=0 (14 bits),
	// alpha_hint=0 (1 bit), version=0 (3 bits)
	// Total: 40 bits = 5 bytes
	var buf bytes.Buffer
	bw := New(&buf)
	bw.WriteBits(0x2F, 8)  // magic
	bw.WriteBits(0, 14)    // width-1
	bw.WriteBits(0, 14)    // height-1
	bw.WriteBits(0, 1)     // alpha_hint
	bw.WriteBits(0, 3)     // version
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	// 0x2F in 8 bits, then 32 zero bits.
	want := []byte{0x2F, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFlush_byteAligned(t *testing.T) {
	var buf bytes.Buffer
	bw := New(&buf)
	bw.WriteBits(0xAB, 8)
	bw.WriteBits(0xCD, 8)
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	want := []byte{0xAB, 0xCD}
	if !bytes.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestWriteBits_largeBatch(t *testing.T) {
	// Write enough data to force buffer flushes.
	var buf bytes.Buffer
	bw := New(&buf)
	for i := 0; i < 8192; i++ {
		bw.WriteBits(uint32(i&0xFF), 8)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()
	if len(got) != 8192 {
		t.Fatalf("expected 8192 bytes, got %d", len(got))
	}
	for i, b := range got {
		if b != byte(i&0xFF) {
			t.Errorf("byte %d: got 0x%02X, want 0x%02X", i, b, byte(i&0xFF))
			break
		}
	}
}
