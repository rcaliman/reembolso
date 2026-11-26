//go:build ignore

// Gera os ícones da aplicação usando somente a biblioteca padrão.
// Executar da raiz do projeto: go run scripts/generate-icons.go
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

func segment(x, y, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	t := math.Max(0, math.Min(1, ((x-ax)*dx+(y-ay)*dy)/(dx*dx+dy*dy)))
	return math.Hypot(x-ax-t*dx, y-ay-t*dy)
}
func pixel(x, y float64) color.RGBA {
	green := color.RGBA{35, 94, 73, 255}
	// O documento permanece na área central segura, inclusive no ícone maskable.
	dx := math.Max(math.Abs(x-.5)-.195, 0)
	dy := math.Max(math.Abs(y-.5)-.245, 0)
	if math.Hypot(dx, dy) > .025 {
		return green
	}
	for _, line := range []float64{.35, .43} {
		if x >= .35 && x <= .65 && math.Abs(y-line) < .014 {
			return green
		}
	}
	if segment(x, y, .38, .59, .47, .68) < .02 || segment(x, y, .47, .68, .64, .52) < .02 {
		return green
	}
	return color.RGBA{255, 255, 255, 255}
}
func main() {
	for _, icon := range []struct {
		name string
		size int
	}{{"icon-192", 192}, {"icon-512", 512}, {"icon-maskable-512", 512}, {"apple-touch-icon", 180}} {
		img := image.NewRGBA(image.Rect(0, 0, icon.size, icon.size))
		for y := 0; y < icon.size; y++ {
			for x := 0; x < icon.size; x++ {
				var r, g, b uint32
				// Supersampling mantém as bordas suaves em todos os tamanhos.
				for sy := 0; sy < 4; sy++ {
					for sx := 0; sx < 4; sx++ {
						c := pixel((float64(x)+(float64(sx)+.5)/4)/float64(icon.size), (float64(y)+(float64(sy)+.5)/4)/float64(icon.size))
						r += uint32(c.R)
						g += uint32(c.G)
						b += uint32(c.B)
					}
				}
				img.SetRGBA(x, y, color.RGBA{uint8(r / 16), uint8(g / 16), uint8(b / 16), 255})
			}
		}
		f, err := os.Create("web/" + icon.name + ".png")
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
