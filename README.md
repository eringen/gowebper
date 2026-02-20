# gowebper

A pure Go library for encoding images to WebP (VP8L) format with configurable compression levels and optional lossy quality control.

## Overview

gowebper converts `image.Image` values to WebP files using the VP8L bitstream. It is implemented from scratch in pure Go with no external dependencies, not even `golang.org/x/image`. Compression effort is controlled by a `Level` (0-9) and an optional `Quality` (0-100) for lossy pre-quantization.

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

    // Lossless (default)
    err := gowebper.Encode(out, img, &gowebper.Options{Level: gowebper.LevelDefault})
    if err != nil {
        panic(err)
    }
}
```

### Lossy encoding

Set `Quality` to a value between 1 and 100. Lower values produce smaller files with more visible loss. Alpha is never modified.

```go
out, _ := os.Create("lossy.webp")
defer out.Close()

err := gowebper.Encode(out, img, &gowebper.Options{
    Level:   gowebper.LevelDefault,
    Quality: 50,
})
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

## Quality (Lossy)

The `Quality` option (0-100) controls near-lossless pre-quantization:

| Quality | Behaviour |
|---------|-----------|
| 0 | Fully lossless (default) |
| 1-25 | Heavy quantization, smallest files |
| 25-75 | Moderate quantization, good balance |
| 75-99 | Light quantization, near-lossless |
| 100 | Minimal rounding, virtually lossless |

Quality works by rounding RGB channel values to fewer significant bits before encoding. The VP8L bitstream itself remains lossless — the loss happens in the pre-processing step. Alpha is never modified. Quality and Level are orthogonal and can be combined freely.

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
