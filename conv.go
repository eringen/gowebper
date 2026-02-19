//go:build ignore

package main

import (
	"image/png"
	"log"
	"os"

	"github.com/eringen/gowebper"
)

func main() {
	f, err := os.Open("test2.png")
	if err != nil {
		log.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}

	out, err := os.Create("test2.webp")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	err = gowebper.Encode(out, img, &gowebper.Options{
		Level:   9,
		Quality: 50,
	})
	if err != nil {
		log.Fatal(err)
	}
}
