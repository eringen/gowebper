// Package bitwriter implements an LSB-first (least-significant-bit first) bit packer
// that writes to an underlying io.Writer. VP8L bitstreams use LSB-first ordering.
package bitwriter

import "io"

const bufSize = 4096

// BitWriter packs bits LSB-first into an internal buffer and flushes to w.
type BitWriter struct {
	w    io.Writer
	buf  [bufSize]byte
	pos  int    // next byte to write in buf
	bits uint64 // pending bits
	nBits int   // number of valid bits in bits
	err  error
}

// New returns a new BitWriter writing to w.
func New(w io.Writer) *BitWriter {
	return &BitWriter{w: w}
}

// WriteBits writes the low n bits of val, LSB first.
// n must be in [0, 32].
func (bw *BitWriter) WriteBits(val uint32, n int) {
	if bw.err != nil || n == 0 {
		return
	}
	bw.bits |= uint64(val) << bw.nBits
	bw.nBits += n
	for bw.nBits >= 8 {
		bw.buf[bw.pos] = byte(bw.bits)
		bw.pos++
		bw.bits >>= 8
		bw.nBits -= 8
		if bw.pos == bufSize {
			bw.flushBuf()
		}
	}
}

// WriteBit writes a single bit (0 or 1), LSB first.
func (bw *BitWriter) WriteBit(val int) {
	bw.WriteBits(uint32(val&1), 1)
}

// Flush writes any remaining buffered bits (zero-padded to byte boundary) to w.
// Must be called at the end of encoding.
func (bw *BitWriter) Flush() error {
	if bw.err != nil {
		return bw.err
	}
	if bw.nBits > 0 {
		bw.buf[bw.pos] = byte(bw.bits)
		bw.pos++
		bw.bits = 0
		bw.nBits = 0
	}
	bw.flushBuf()
	return bw.err
}

// Err returns the first write error encountered.
func (bw *BitWriter) Err() error {
	return bw.err
}

func (bw *BitWriter) flushBuf() {
	if bw.err != nil || bw.pos == 0 {
		return
	}
	_, bw.err = bw.w.Write(bw.buf[:bw.pos])
	bw.pos = 0
}
