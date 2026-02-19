//go:build ignore

package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"os"

	"github.com/eringen/gowebper"
)

func main() {
	// Solid 2x2 red image (palette path, simplest possible)
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	out, _ := os.Create("/tmp/min_solid.webp")
	err := gowebper.Encode(out, img, &gowebper.Options{Level: 0})
	out.Close()
	if err != nil { log.Fatal(err) }
	fmt.Println("wrote min_solid.webp")

	// 4x4 gradient (more colors, still palette)
	img2 := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img2.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 85), G: uint8(y * 85), B: 128, A: 255,
			})
		}
	}
	out2, _ := os.Create("/tmp/min_gradient.webp")
	err = gowebper.Encode(out2, img2, &gowebper.Options{Level: 0})
	out2.Close()
	if err != nil { log.Fatal(err) }
	fmt.Println("wrote min_gradient.webp")

	// 1x1 single pixel
	img3 := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img3.SetNRGBA(0, 0, color.NRGBA{R: 42, G: 43, B: 44, A: 255})
	out3, _ := os.Create("/tmp/min_1x1.webp")
	err = gowebper.Encode(out3, img3, &gowebper.Options{Level: 0})
	out3.Close()
	if err != nil { log.Fatal(err) }
	fmt.Println("wrote min_1x1.webp")
}
