// Created by Jobs on 2026年11月18日，星期三.
// This deterministic fixture has two distinct frames and is not a product visual asset.
package main

import (
	"image"
	"image/color"
	"image/gif"
	"os"
)

func main() {
	palette := color.Palette{color.RGBA{200, 20, 20, 255}, color.RGBA{20, 20, 200, 255}}
	first, second := image.NewPaletted(image.Rect(0, 0, 4, 4), palette), image.NewPaletted(image.Rect(0, 0, 4, 4), palette)
	for i := range second.Pix {
		second.Pix[i] = 1
	}
	f, err := os.OpenFile(os.Args[1], os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	if err = gif.EncodeAll(f, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{9, 21}, LoopCount: 0}); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
}
