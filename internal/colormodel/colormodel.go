// Package colormodel converts image.Image values to packed ARGB uint32 slices.
package colormodel

import "image"

// ToARGB converts an image.Image to a slice of packed uint32 ARGB values.
// Each pixel is stored as 0xAARRGGBB in the order (x=0,y=0), (x=1,y=0), ...
// The slice has length width*height.
//
// For common standard-library image types the raw non-premultiplied values are
// extracted directly. For unknown types the caller receives premultiplied values
// (i.e. what image.Color.RGBA() returns shifted right by 8).
func ToARGB(img image.Image) []uint32 {
	b := img.Bounds()
	w := b.Max.X - b.Min.X
	h := b.Max.Y - b.Min.Y
	if w <= 0 || h <= 0 {
		return nil
	}
	out := make([]uint32, w*h)

	switch v := img.(type) {
	case *image.NRGBA:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := (y-b.Min.Y)*w
			for x := b.Min.X; x < b.Max.X; x++ {
				i := v.PixOffset(x, y)
				p := v.Pix[i : i+4]
				out[off+(x-b.Min.X)] = uint32(p[3])<<24 | uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
			}
		}
	case *image.RGBA:
		// RGBA stores premultiplied values; we store them as-is since
		// premultiplied ARGB is the natural VP8L representation for such images.
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := (y-b.Min.Y)*w
			for x := b.Min.X; x < b.Max.X; x++ {
				i := v.PixOffset(x, y)
				p := v.Pix[i : i+4]
				out[off+(x-b.Min.X)] = uint32(p[3])<<24 | uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
			}
		}
	case *image.Gray:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := (y-b.Min.Y)*w
			for x := b.Min.X; x < b.Max.X; x++ {
				i := v.PixOffset(x, y)
				lum := uint32(v.Pix[i])
				out[off+(x-b.Min.X)] = 0xFF<<24 | lum<<16 | lum<<8 | lum
			}
		}
	case *image.NRGBA64:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := (y-b.Min.Y)*w
			for x := b.Min.X; x < b.Max.X; x++ {
				i := v.PixOffset(x, y)
				p := v.Pix[i : i+8]
				// High byte of each 16-bit component.
				out[off+(x-b.Min.X)] = uint32(p[6])<<24 | uint32(p[0])<<16 | uint32(p[2])<<8 | uint32(p[4])
			}
		}
	case *image.YCbCr:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			off := (y-b.Min.Y)*w
			for x := b.Min.X; x < b.Max.X; x++ {
				yi := v.YOffset(x, y)
				ci := v.COffset(x, y)
				r, g, bv := ycbcrToRGB(v.Y[yi], v.Cb[ci], v.Cr[ci])
				out[off+(x-b.Min.X)] = 0xFF<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(bv)
			}
		}
	default:
		// Generic slow path: calls image.Color.RGBA() which returns premultiplied values.
		idx := 0
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r32, g32, b32, a32 := img.At(x, y).RGBA()
				out[idx] = (a32>>8)<<24 | (r32>>8)<<16 | (g32>>8)<<8 | (b32 >> 8)
				idx++
			}
		}
	}
	return out
}

// HasAlpha reports whether any pixel in pixels has alpha < 255.
func HasAlpha(pixels []uint32) bool {
	for _, p := range pixels {
		if (p >> 24) != 0xff {
			return true
		}
	}
	return false
}

// ycbcrToRGB converts a YCbCr triplet to RGB using integer arithmetic.
func ycbcrToRGB(y, cb, cr uint8) (r, g, b uint8) {
	yy := int32(y) * 65793
	cb1 := int32(cb) - 128
	cr1 := int32(cr) - 128
	r32 := (yy + 91881*cr1 + 1<<15) >> 16
	g32 := (yy - 22554*cb1 - 46802*cr1 + 1<<15) >> 16
	b32 := (yy + 116130*cb1 + 1<<15) >> 16
	r = clamp8(r32)
	g = clamp8(g32)
	b = clamp8(b32)
	return
}

func clamp8(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
