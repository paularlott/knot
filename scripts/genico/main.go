// Assembles a multi-size .ico (32-bit BGRA DIB entries + zero AND masks)
// from input PNGs; sizes come from the PNG dimensions.
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
)

func dib(img *image.NRGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	xorRow := w * 4
	andRow := ((w + 31) / 32) * 4

	out := make([]byte, 40+xorRow*h+andRow*h)
	binary.LittleEndian.PutUint32(out[0:], 40)      // biSize
	binary.LittleEndian.PutUint32(out[4:], uint32(w))
	binary.LittleEndian.PutUint32(out[8:], uint32(h*2)) // XOR + AND
	binary.LittleEndian.PutUint16(out[12:], 1)      // planes
	binary.LittleEndian.PutUint16(out[14:], 32)     // 32bpp

	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		dst := out[40+(h-1-y)*xorRow : 40+(h-1-y)*xorRow+xorRow]
		for x := 0; x < w; x++ {
			dst[x*4+0] = row[x*4+2] // B
			dst[x*4+1] = row[x*4+1] // G
			dst[x*4+2] = row[x*4+0] // R
			dst[x*4+3] = row[x*4+3] // A
		}
	}
	return out
}

func main() {
	out, err := os.Create(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()

	var entries []byte
	var images []byte
	count := len(os.Args) - 2

	dir := make([]byte, 6)
	binary.LittleEndian.PutUint16(dir[0:], 0)
	binary.LittleEndian.PutUint16(dir[2:], 1)
	binary.LittleEndian.PutUint16(dir[4:], uint16(count))

	for _, path := range os.Args[2:] {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, path, err)
			os.Exit(1)
		}
		nrgba, ok := img.(*image.NRGBA)
		if !ok {
			fmt.Fprintln(os.Stderr, path, "not NRGBA")
			os.Exit(1)
		}

		blob := dib(nrgba)
		w, h := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

		var e [16]byte
		if w >= 256 {
			e[0] = 0
		} else {
			e[0] = byte(w)
		}
		if h >= 256 {
			e[1] = 0
		} else {
			e[1] = byte(h)
		}
		e[4] = 1                    // planes
		e[6], e[7] = 32, 0          // bit count
		binary.LittleEndian.PutUint32(e[8:], uint32(len(blob)))
		binary.LittleEndian.PutUint32(e[12:], uint32(6+16*count+len(images)))
		entries = append(entries, e[:]...)

		images = append(images, blob...)
	}

	out.Write(dir)
	out.Write(entries)
	out.Write(images)
}
