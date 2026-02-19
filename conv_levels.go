//go:build ignore

package main

import (
	"fmt"
	"image/png"
	"log"
	"os"

	"github.com/eringen/gowebper"
)

func main() {
	f, err := os.Open("test.png")
	if err != nil {
		log.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		log.Fatal(err)
	}
	for _, level := range []int{0, 1, 3, 6, 9} {
		name := fmt.Sprintf("/tmp/test_l%d.webp", level)
		out, err := os.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		err = gowebper.Encode(out, img, &gowebper.Options{Level: level})
		out.Close()
		if err != nil {
			log.Fatalf("level %d: %v", level, err)
		}
		fmt.Printf("level %d: wrote %s\n", level, name)
	}
}
