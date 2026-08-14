// iconthin thins the strokes of a monochrome icon without altering its
// geometry: it thresholds the alpha channel, erodes the resulting mask by a
// radius (repeated 3x3 min-filter, an approximation of a disk), then
// downsamples with area averaging for smooth antialiased edges. Output is
// square, -size pixels wide, preserving the input colour.
//
// Usage: iconthin -size 44 -erode 10 input.png output.png
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
)

func main() {
	size := flag.Int("size", 44, "output width/height in pixels")
	erode := flag.Int("erode", 10, "erosion radius in input-resolution pixels")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: iconthin -size 44 -erode 10 input.png output.png")
		os.Exit(2)
	}

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	// Binary mask from the alpha channel.
	mask := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a >= 0x8000 {
				mask[y*w+x] = 1
			}
		}
	}

	// Erode: each round is a 3x3 min-filter; r rounds approximate a disk of
	// radius r. Edge pixels read as outside, so strokes also recede from the
	// canvas edge.
	for round := 0; round < *erode; round++ {
		next := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var m byte = 1
				for dy := -1; dy <= 1 && m == 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						ny, nx := y+dy, x+dx
						if ny < 0 || ny >= h || nx < 0 || nx >= w || mask[ny*w+nx] == 0 {
							m = 0
							break
						}
					}
				}
				next[y*w+x] = m
			}
		}
		mask = next
	}

	// Area-average downsample of the mask for antialiasing; colour is taken
	// from the input (monochrome throughout).
	out := image.NewNRGBA(image.Rect(0, 0, *size, *size))
	scale := float64(w) / float64(*size)
	for y := 0; y < *size; y++ {
		for x := 0; x < *size; x++ {
			x0 := float64(x) * scale
			y0 := float64(y) * scale
			x1 := float64(x+1) * scale
			y1 := float64(y+1) * scale
			var cov, n float64
			for sy := int(y0); sy < int(y1+0.999999); sy++ {
				if sy >= h {
					break
				}
				for sx := int(x0); sx < int(x1+0.999999); sx++ {
					if sx >= w {
						break
					}
					fx := 1.0
					if lo, hi := x0, x1; float64(sx) < lo || float64(sx)+1 > hi {
						fx = min(float64(sx)+1, hi) - max(float64(sx), lo)
					}
					fy := 1.0
					if lo, hi := y0, y1; float64(sy) < lo || float64(sy)+1 > hi {
						fy = min(float64(sy)+1, hi) - max(float64(sy), lo)
					}
					cov += fx * fy * float64(mask[sy*w+sx])
					n += fx * fy
				}
			}
			o := y*(*size) + x
			c := src.At(b.Min.X+int(x0+scale/2), b.Min.Y+int(y0+scale/2))
			cr, cg, cb, _ := c.RGBA()
			out.Pix[o*4+0] = byte(cr >> 8)
			out.Pix[o*4+1] = byte(cg >> 8)
			out.Pix[o*4+2] = byte(cb >> 8)
			out.Pix[o*4+3] = byte(cov / n * 255)
		}
	}

	o, err := os.Create(flag.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer o.Close()
	if err := png.Encode(o, out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
