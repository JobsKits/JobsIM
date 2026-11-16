package jobsim

import (
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
)

// The client filename and MIME never authorize an audio/video type. Inspect the
// ISO BMFF track handlers so AAC/M4A is not mislabeled as a video container.
func classifyStoredMedia(path, sniffed string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return "", e
	}
	prefix := make([]byte, 32)
	n, _ := f.ReadAt(prefix, 0)
	prefix = prefix[:n]
	if len(prefix) >= 12 && string(prefix[4:8]) == "ftyp" {
		audio, video, e := bmffTracks(f, stat.Size())
		if e != nil {
			return "", fail(400, "INVALID_MEDIA_CONTAINER", "Invalid MPEG4 track structure")
		}
		if video {
			return "video/mp4", nil
		}
		if audio {
			return "audio/mp4", nil
		}
		return "application/octet-stream", nil
	}
	if strings.HasPrefix(sniffed, "image/") {
		if _, e = f.Seek(0, 0); e != nil {
			return "", e
		}
		cfg, format, e := image.DecodeConfig(f)
		if e == nil {
			if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 40000000 {
				return "", fail(413, "IMAGE_PIXELS", "Image pixel limit exceeded")
			}
			if format == "gif" {
				if e = validateGIF(f, stat.Size()); e != nil {
					return "", e
				}
			}
			return sniffed, nil
		}
		if sniffed == "image/webp" {
			width, height, ok := webpDimensions(prefix)
			if !ok || int64(width)*int64(height) > 40000000 {
				return "", fail(400, "INVALID_WEBP", "Unsupported or oversized WebP dimensions")
			}
			return sniffed, nil
		}
		return "application/octet-stream", nil
	}
	return sniffed, nil
}
func bmffTracks(r io.ReaderAt, size int64) (audio, video bool, err error) {
	boxes := 0
	ftyp := false
	var walk func(int64, int64, string, int) error
	walk = func(start, end int64, path string, depth int) error {
		if depth > 8 {
			return fmt.Errorf("depth limit")
		}
		for pos := start; pos < end; {
			boxes++
			if boxes > 20000 || end-pos < 8 {
				return fmt.Errorf("box limit or truncated box")
			}
			header := make([]byte, 16)
			if _, e := r.ReadAt(header[:8], pos); e != nil {
				return e
			}
			length := int64(binary.BigEndian.Uint32(header[:4]))
			kind := string(header[4:8])
			head := int64(8)
			if length == 1 {
				if _, e := r.ReadAt(header[8:], pos+8); e != nil {
					return e
				}
				n := binary.BigEndian.Uint64(header[8:])
				if n > uint64(end-pos) {
					return fmt.Errorf("oversized box")
				}
				length = int64(n)
				head = 16
			} else if length == 0 {
				length = end - pos
			}
			if length < head || length > end-pos {
				return fmt.Errorf("invalid box bounds")
			}
			nextPath := path + "/" + kind
			if path == "" && kind == "ftyp" {
				if length < head+8 {
					return fmt.Errorf("invalid ftyp")
				}
				ftyp = true
			}
			if nextPath == "/moov" || nextPath == "/moov/trak" || nextPath == "/moov/trak/mdia" {
				if e := walk(pos+head, pos+length, nextPath, depth+1); e != nil {
					return e
				}
			}
			if path == "/moov/trak/mdia" && kind == "hdlr" {
				if length < head+12 {
					return fmt.Errorf("invalid handler")
				}
				payload := make([]byte, 12)
				if _, e := r.ReadAt(payload, pos+head); e != nil {
					return e
				}
				switch string(payload[8:12]) {
				case "soun":
					audio = true
				case "vide":
					video = true
				}
			}
			pos += length
		}
		return nil
	}
	err = walk(0, size, "", 0)
	if err == nil && !ftyp {
		err = fmt.Errorf("ftyp absent")
	}
	return
}
func webpDimensions(p []byte) (int, int, bool) {
	if len(p) < 30 || string(p[:4]) != "RIFF" || string(p[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(p[12:16]) {
	case "VP8X":
		return 1 + int(p[24]) + int(p[25])<<8 + int(p[26])<<16, 1 + int(p[27]) + int(p[28])<<8 + int(p[29])<<16, true
	case "VP8L":
		if p[20] != 0x2f {
			return 0, 0, false
		}
		return 1 + int(p[21]) + int(p[22]&63)<<8, 1 + int(p[22]>>6) + int(p[23])<<2 + int(p[24]&15)<<10, true
	case "VP8 ":
		if p[23] != 0x9d || p[24] != 1 || p[25] != 0x2a {
			return 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(p[26:28]) & 16383), int(binary.LittleEndian.Uint16(p[28:30]) & 16383), true
	}
	return 0, 0, false
}
func validateGIF(r io.ReaderAt, size int64) error {
	b := make([]byte, size)
	if _, e := r.ReadAt(b, 0); e != nil {
		return e
	}
	if len(b) < 13 {
		return fail(400, "INVALID_GIF", "Invalid GIF")
	}
	pos := 13
	if b[10]&0x80 != 0 {
		pos += 3 * (1 << ((b[10] & 7) + 1))
	}
	frames := 0
	pixels := int64(0)
	duration := 0
	skip := func() bool {
		for {
			if pos >= len(b) {
				return false
			}
			n := int(b[pos])
			pos++
			if n == 0 {
				return true
			}
			if n > len(b)-pos {
				return false
			}
			pos += n
		}
	}
	for pos < len(b) {
		kind := b[pos]
		pos++
		switch kind {
		case 0x3b:
			if frames == 0 {
				return fail(400, "INVALID_GIF", "GIF has no frames")
			}
			return nil
		case 0x21:
			if pos >= len(b) {
				break
			}
			label := b[pos]
			pos++
			if label == 0xf9 && pos+5 <= len(b) && b[pos] == 4 {
				duration += int(binary.LittleEndian.Uint16(b[pos+2 : pos+4]))
				if duration > 6000 {
					return fail(413, "GIF_DURATION", "GIF duration limit exceeded")
				}
			}
			if !skip() {
				return fail(400, "INVALID_GIF", "Invalid GIF extension")
			}
		case 0x2c:
			if pos+9 > len(b) {
				return fail(400, "INVALID_GIF", "Invalid GIF frame")
			}
			width := int64(binary.LittleEndian.Uint16(b[pos+4 : pos+6]))
			height := int64(binary.LittleEndian.Uint16(b[pos+6 : pos+8]))
			pixels += width * height
			frames++
			packed := b[pos+8]
			pos += 9
			if frames > 500 || pixels > 100000000 {
				return fail(413, "GIF_FRAMES", "GIF frame/pixel limit exceeded")
			}
			if packed&0x80 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			pos++
			if pos > len(b) || !skip() {
				return fail(400, "INVALID_GIF", "Invalid GIF image data")
			}
		default:
			return fail(400, "INVALID_GIF", "Unexpected GIF block")
		}
	}
	return fail(400, "INVALID_GIF", "Truncated GIF")
}
