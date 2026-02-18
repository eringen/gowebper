# gowebper

A pure Go library for encoding images to lossless WebP (VP8L) format with configurable compression levels.

## Overview

gowebper converts `image.Image` values to lossless WebP files. It implements the VP8L bitstream from scratch with no external dependencies, not even `golang.org/x/image`. Compression effort is controlled by a single integer level from 0 (fastest) to 9 (best).

## Installation

```
go get github.com/eringen/gowebper
```

## Usage

### Encode to a writer

```go
import (
    "image/png"
    "os"

    "github.com/eringen/gowebper"
)

func main() {
    f, _ := os.Open("input.png")
    defer f.Close()
    img, _ := png.Decode(f)

    out, _ := os.Create("output.webp")
    defer out.Close()

    err := gowebper.Encode(out, img, &gowebper.Options{Level: gowebper.LevelDefault})
    if err != nil {
        panic(err)
    }
}
```

### Encode to bytes

```go
import (
    "github.com/eringen/gowebper"
)

func encode(img image.Image) ([]byte, error) {
    return gowebper.EncodeToBytes(img, &gowebper.Options{Level: gowebper.LevelBest})
}
```

### Reusable Encoder

Amortises allocations across multiple calls:

```go
import (
    "github.com/eringen/gowebper"
)

func encodeMany(images []image.Image, w []io.Writer) error {
    enc := gowebper.NewEncoder(&gowebper.Options{Level: gowebper.LevelDefault})
    for i, img := range images {
        if err := enc.Encode(w[i], img); err != nil {
            return err
        }
    }
    return nil
}
```

## Compression Levels

| Level | Transforms | LZ77 window | Description |
|-------|-----------|-------------|-------------|
| 0 | none | none | Fastest, largest file |
| 1 | SubtractGreen | 1024 | Minimal compression |
| 2 | SubtractGreen | 2048 | Low compression |
| 3 | SubtractGreen | 4096 | Below average |
| 4 | SubtractGreen + Predictor | 8192 | Average |
| 5 | SubtractGreen + Predictor | 16384 | Above average |
| 6 | SubtractGreen + Predictor | 32768 | Default |
| 7 | SubtractGreen + Predictor + CrossColor | 65536 | Good |
| 8 | SubtractGreen + Predictor + CrossColor | 262144 | Better (with color cache) |
| 9 | SubtractGreen + Predictor + CrossColor | 1048576 | Best, slowest |

Convenience constants: `LevelFastest = 0`, `LevelDefault = 6`, `LevelBest = 9`.

The palette transform is attempted at all levels and applied only when the image has 256 or fewer distinct colors and the palette representation is smaller than the non-palette encoding.

## Format Notes

VP8L is the lossless sub-format of the WebP container. It uses:

- A RIFF container with a WEBP FOURCC and a VP8L chunk
- LSB-first canonical Huffman coding
- LZ77 backward references with a 2D spatial distance table
- Four optional pre-processing transforms: SubtractGreen, Predictor, CrossColor, Palette

The output is readable by any WebP viewer including Chrome, macOS Preview, and ffmpeg.

## API Reference

```go
func Encode(w io.Writer, m image.Image, opts *Options) error
func EncodeToBytes(m image.Image, opts *Options) ([]byte, error)
func NewEncoder(opts *Options) *Encoder
func (e *Encoder) Encode(w io.Writer, m image.Image) error
```

## Contributing

Contributions are welcome. Please open an issue before submitting a large change. Run `go test ./...` and `go vet ./...` before sending a pull request.

## License

MIT
