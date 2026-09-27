// Command icon draws Seaglass's app icon (a water drop with a play
// triangle) into build/appicon.png and build/windows/icon.ico. Every size
// is drawn from the vector shape, so small icons stay sharp. Run from the
// repository root:
//
//	go run ./tools/icon
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"golang.org/x/image/vector"
)

func main() {
	if err := writePNG("build/appicon.png", draw(1024)); err != nil {
		log.Fatal(err)
	}
	if err := writeICO("build/windows/icon.ico", []int{256, 128, 64, 48, 32, 24, 16}); err != nil {
		log.Fatal(err)
	}
}

// draw renders the mark at size×size. It is designed on a 30×30 grid, as
// the SVG in the interface is.
func draw(size int) *image.RGBA {
	s := float64(size)
	scale := s * 0.88 / 22.7
	ox := (s-17*scale)/2 - 6.5*scale
	oy := (s-22.7*scale)/2 - 3.5*scale
	p := func(x, y float64) (float32, float32) { return float32(ox + x*scale), float32(oy + y*scale) }

	drop := vector.NewRasterizer(size, size)
	const k = 0.5523 * 8.5 // a quarter circle as a cubic
	drop.MoveTo(p(15, 3.5))
	c := func(x1, y1, x2, y2, x, y float64) {
		a, b := p(x1, y1)
		cc, d := p(x2, y2)
		e, f := p(x, y)
		drop.CubeTo(a, b, cc, d, e, f)
	}
	c(19.8, 9.1, 23.5, 13.5, 23.5, 17.7)
	c(23.5, 17.7+k, 15+k, 26.2, 15, 26.2)
	c(15-k, 26.2, 6.5, 17.7+k, 6.5, 17.7)
	c(6.5, 13.5, 10.2, 9.1, 15, 3.5)
	drop.ClosePath()

	// Light aqua at the top, deep blue at the bottom.
	grad := image.NewRGBA(image.Rect(0, 0, size, size))
	top, bottom := color.RGBA{0x7c, 0xe8, 0xf6, 0xff}, color.RGBA{0x10, 0x6f, 0xc4, 0xff}
	for y := 0; y < size; y++ {
		t := float64(y) / s
		col := color.RGBA{lerp(top.R, bottom.R, t), lerp(top.G, bottom.G, t), lerp(top.B, bottom.B, t), 0xff}
		for x := 0; x < size; x++ {
			grad.SetRGBA(x, y, col)
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, size, size))
	drop.Draw(out, out.Bounds(), grad, image.Point{})

	play := vector.NewRasterizer(size, size)
	play.MoveTo(p(12.6, 13.9))
	play.LineTo(p(12.6, 21.9))
	play.LineTo(p(19.1, 17.9))
	play.ClosePath()
	play.Draw(out, out.Bounds(), image.NewUniform(color.White), image.Point{})
	return out
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5) }

func writePNG(path string, img image.Image) error {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// writeICO writes a Windows icon holding one PNG-compressed image per size.
func writeICO(path string, sizes []int) error {
	var pngs [][]byte
	for _, sz := range sizes {
		var b bytes.Buffer
		if err := png.Encode(&b, draw(sz)); err != nil {
			return err
		}
		pngs = append(pngs, b.Bytes())
	}
	var out bytes.Buffer
	le := binary.LittleEndian
	_ = binary.Write(&out, le, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, sz := range sizes {
		dim := uint8(sz)
		if sz >= 256 {
			dim = 0
		}
		_ = binary.Write(&out, le, struct {
			W, H, Colors, Reserved uint8
			Planes, BitCount       uint16
			Size, Offset           uint32
		}{dim, dim, 0, 0, 1, 32, uint32(len(pngs[i])), uint32(offset)})
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}
