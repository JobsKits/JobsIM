package jobsim

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestAvatarDecodeAndCanonicalOutputs(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			source.SetNRGBA(x, y, color.NRGBA{220, 30, 10, 128})
		}
	}
	var encoded bytes.Buffer
	if e := png.Encode(&encoded, source); e != nil {
		t.Fatal(e)
	}
	decoded, e := decodeStaticAvatar(encoded.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	for _, size := range []int{512, 128} {
		out, e := encodeAvatarSquare(decoded, size)
		if e != nil {
			t.Fatal(e)
		}
		img, e := jpeg.Decode(bytes.NewReader(out))
		if e != nil {
			t.Fatal(e)
		}
		if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Fatal("canonical dimensions differ")
		}
		r, g, b, _ := img.At(size/2, size/2).RGBA()
		if r < 50000 || g < 30000 || b < 30000 {
			t.Fatal("alpha was not flattened on a white background")
		}
	}
	if _, e = decodeStaticAvatar([]byte("untrusted filename.png")); e == nil {
		t.Fatal("non-image accepted")
	}
	if _, e = decodeStaticAvatar([]byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")); e == nil {
		t.Fatal("dynamic avatar accepted")
	}
	// APNG is rejected from its structured animation-control chunk, even when the default frame is valid PNG.
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload, 2)
	chunk := append([]byte("acTL"), payload...)
	control := make([]byte, 4)
	binary.BigEndian.PutUint32(control, 8)
	control = append(control, chunk...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(chunk))
	control = append(control, crc...)
	animated := append(append(append([]byte{}, encoded.Bytes()[:33]...), control...), encoded.Bytes()[33:]...)
	if _, e = decodeStaticAvatar(animated); e == nil {
		t.Fatal("APNG accepted as a static avatar")
	}
}

func TestAvatarFilePublishIsIdempotentAndNeverOverwrites(t *testing.T) {
	a := &App{Config: Config{StateDir: t.TempDir(), Key: bytes.Repeat([]byte{42}, 32)}}
	id := a.avatarObjectID("owner", "stable-operation", "root")
	if id == a.avatarObjectID("other-owner", "stable-operation", "root") || id == a.avatarObjectID("owner", "stable-operation", "thumbnail") {
		t.Fatal("avatar output identity escaped its owner or kind")
	}
	if e := a.writeAvatarFile(id, []byte("first complete canonical bytes")); e != nil {
		t.Fatal(e)
	}
	if e := a.writeAvatarFile(id, []byte("first complete canonical bytes")); e != nil {
		t.Fatal(e)
	}
	if e := a.writeAvatarFile(id, []byte("different output")); e == nil {
		t.Fatal("prior output overwritten")
	}
}
