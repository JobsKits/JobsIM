// Created by Jobs on 2026年11月18日，星期三.
package main

import (
	"bytes"
	"encoding/json"
	"image/gif"
	"os"
)

func main() {
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	img, err := gif.DecodeAll(bytes.NewReader(raw))
	if err != nil {
		panic(err)
	}
	if len(img.Image) != 2 || img.Delay[0] != 9 || img.Delay[1] != 21 || img.LoopCount != 0 || img.Config.Width != 512 || img.Config.Height != 512 {
		panic("actual HTTPS animation identity changed")
	}
	r0, _, b0, _ := img.Image[0].At(250, 250).RGBA()
	r1, _, b1, _ := img.Image[1].At(250, 250).RGBA()
	if r0 <= b0 || b1 <= r1 {
		panic("actual frames not distinct")
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "passed", "frames": len(img.Image), "delay": img.Delay, "size": 512, "actualHTTPSBytes": true})
}
