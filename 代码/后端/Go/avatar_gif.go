// avatar_gif.go
// JobsIM
// Created by Jobs on 2026年11月18日，星期三.
package jobsim

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
)

const avatarGIFFrameLimit = 120
const avatarGIFPixelLimit = 32000000
const avatarGIFDelayLimit = 60000

func isGIFAvatar(content []byte) bool {
	return bytes.HasPrefix(content, []byte("GIF87a")) || bytes.HasPrefix(content, []byte("GIF89a"))
}

// Bound all decoded frame allocations before gif.DecodeAll allocates them.
func inspectAvatarGIF(content []byte) error {
	bad := func() error { return fail(400, "AVATAR_FORMAT", "Malformed GIF avatar") }
	if len(content) < 13 || !isGIFAvatar(content) {
		return bad()
	}
	width, height := int(binary.LittleEndian.Uint16(content[6:8])), int(binary.LittleEndian.Uint16(content[8:10]))
	if width < 1 || height < 1 || int64(width)*int64(height) > 16000000 {
		return fail(413, "AVATAR_PIXELS", "GIF canvas permits at most16 megapixels")
	}
	pos := 13
	if content[10]&0x80 != 0 {
		pos += 3 * (1 << ((content[10] & 7) + 1))
	}
	frames, pixels := 0, int64(0)
	skipBlocks := func() bool {
		for pos < len(content) {
			size := int(content[pos])
			pos++
			if size == 0 {
				return true
			}
			if size > len(content)-pos {
				return false
			}
			pos += size
		}
		return false
	}
	for pos < len(content) {
		kind := content[pos]
		pos++
		switch kind {
		case 0x3b:
			if frames < 1 || pos != len(content) {
				return bad()
			}
			return nil
		case 0x21:
			if pos >= len(content) {
				return bad()
			}
			pos++
			if !skipBlocks() {
				return bad()
			}
		case 0x2c:
			if len(content)-pos < 9 {
				return bad()
			}
			x, y := int(binary.LittleEndian.Uint16(content[pos:pos+2])), int(binary.LittleEndian.Uint16(content[pos+2:pos+4]))
			w, h := int(binary.LittleEndian.Uint16(content[pos+4:pos+6])), int(binary.LittleEndian.Uint16(content[pos+6:pos+8]))
			if w < 1 || h < 1 || x+w > width || y+h > height {
				return bad()
			}
			frames++
			pixels += int64(w) * int64(h)
			if frames > avatarGIFFrameLimit || pixels > avatarGIFPixelLimit {
				return fail(413, "AVATAR_ANIMATION_LIMIT", "GIF permits120 frames and32 million total frame pixels")
			}
			packed := content[pos+8]
			pos += 9
			if packed&0x80 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			if pos >= len(content) {
				return bad()
			}
			pos++
			if !skipBlocks() {
				return bad()
			}
		default:
			return bad()
		}
	}
	return bad()
}

func avatarAnimatedSquare(src image.Image, size int) *image.Paletted {
	b := src.Bounds()
	edge := b.Dx()
	if b.Dy() < edge {
		edge = b.Dy()
	}
	ox, oy := b.Min.X+(b.Dx()-edge)/2, b.Min.Y+(b.Dy()-edge)/2
	square := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx, sy := ox+(2*x+1)*edge/(2*size), oy+(2*y+1)*edge/(2*size)
			square.Set(x, y, src.At(sx, sy))
		}
	}
	colors := append(color.Palette{color.Transparent}, palette.WebSafe...)
	output := image.NewPaletted(square.Bounds(), colors)
	draw.FloydSteinberg.Draw(output, output.Bounds(), square, image.Point{})
	return output
}

func encodeAnimatedAvatar(content []byte) ([]byte, []byte, error) {
	if err := inspectAvatarGIF(content); err != nil {
		return nil, nil, err
	}
	source, err := gif.DecodeAll(bytes.NewReader(content))
	if err != nil {
		return nil, nil, fail(400, "AVATAR_DECODE", "GIF frames cannot be decoded")
	}
	var duration int
	for _, delay := range source.Delay {
		duration += delay
	}
	if duration > avatarGIFDelayLimit {
		return nil, nil, fail(413, "AVATAR_ANIMATION_LIMIT", "GIF total frame delay permits at most10 minutes")
	}
	canvas := image.NewRGBA(image.Rect(0, 0, source.Config.Width, source.Config.Height))
	var background color.Color = color.Transparent
	if colors, ok := source.Config.ColorModel.(color.Palette); ok && int(source.BackgroundIndex) < len(colors) {
		background = colors[source.BackgroundIndex]
	}
	// GIF transparency applies per frame. A transparent logical background must
	// stay transparent when a frame requests disposal to that background.
	for _, c := range source.Image[0].Palette {
		_, _, _, alpha := c.RGBA()
		if alpha == 0 {
			background = color.Transparent
			break
		}
	}
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	output := &gif.GIF{LoopCount: source.LoopCount, Delay: append([]int(nil), source.Delay...)}
	var previous *image.RGBA
	var poster []byte
	for index, frame := range source.Image {
		if index > 0 && len(source.Disposal) > index-1 {
			switch source.Disposal[index-1] {
			case gif.DisposalBackground:
				draw.Draw(canvas, source.Image[index-1].Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
			case gif.DisposalPrevious:
				if previous != nil {
					copy(canvas.Pix, previous.Pix)
				}
			}
		}
		if len(source.Disposal) > index && source.Disposal[index] == gif.DisposalPrevious {
			if previous == nil {
				previous = image.NewRGBA(canvas.Bounds())
			}
			copy(previous.Pix, canvas.Pix)
		}
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		if index == 0 {
			poster, err = encodeAvatarSquare(canvas, 128)
			if err != nil {
				return nil, nil, err
			}
		}
		output.Image = append(output.Image, avatarAnimatedSquare(canvas, 512))
		output.Disposal = append(output.Disposal, gif.DisposalBackground)
	}
	output.Config = image.Config{ColorModel: output.Image[0].Palette, Width: 512, Height: 512}
	var encoded bytes.Buffer
	if err = gif.EncodeAll(&encoded, output); err != nil {
		return nil, nil, err
	}
	if encoded.Len() > 2<<20 {
		return nil, nil, fail(413, "AVATAR_OUTPUT_LIMIT", "Canonical animated avatar exceeds2 MiB; choose a shorter or simpler GIF")
	}
	return encoded.Bytes(), poster, nil
}
