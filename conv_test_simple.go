//go:build ignore

package main

import (
	"image"
	"image/color"
	"log"
	"os"

	"github.com/eringen/gowebper"
)

func main() {
	// Test 1: 2x2 image with 4 colors (palette path, ≤256 colors)
	img2x2 := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img2x2.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255}) // red
	img2x2.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 255}) // green
	img2x2.SetNRGBA(0, 1, color.NRGBA{0, 0, 255, 255}) // blue
	img2x2.SetNRGBA(1, 1, color.NRGBA{255, 255, 255, 255}) // white
	writeWebP("test_2x2.webp", img2x2, 0)

	// Test 2: 1x1 solid red
	img1x1 := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img1x1.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	writeWebP("test_1x1.webp", img1x1, 0)

	// Test 3: 20x20 gradient (>256 colors, bypasses palette)
	img20 := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			img20.SetNRGBA(x, y, color.NRGBA{
				uint8(x*12 + y), uint8(y*12 + x), uint8((x + y) * 3), 255,
			})
		}
	}
	writeWebP("test_20x20.webp", img20, 0)

	// Test 4: 4x4 with exactly 2 colors (very small palette)
	img4x4 := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if (x+y)%2 == 0 {
				img4x4.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
			} else {
				img4x4.SetNRGBA(x, y, color.NRGBA{0, 0, 255, 255})
			}
		}
	}
	writeWebP("test_4x4_checkers.webp", img4x4, 0)
}

func writeWebP(name string, img image.Image, level int) {
	f, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	err = gowebper.Encode(f, img, &gowebper.Options{Level: level})
	if err != nil {
		log.Fatalf("%s: %v", name, err)
	}
	log.Printf("wrote %s", name)
}
