package jobsim

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"strings"
	"testing"
	"time"
)

func profileAnimatedFixture(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{color.Transparent, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255}}
	first := image.NewPaletted(image.Rect(0, 0, 4, 4), palette)
	for i := range first.Pix {
		first.Pix[i] = 1
	}
	second := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	for i := range second.Pix {
		second.Pix[i] = 2
	}
	third := image.NewPaletted(image.Rect(2, 2, 4, 4), palette)
	for i := range third.Pix {
		third.Pix[i] = 2
	}
	input := &gif.GIF{Image: []*image.Paletted{first, second, third}, Delay: []int{7, 13, 19}, Disposal: []byte{gif.DisposalNone, gif.DisposalPrevious, gif.DisposalNone}, LoopCount: 2, Config: image.Config{ColorModel: palette, Width: 4, Height: 4}}
	var raw bytes.Buffer
	if err := gif.EncodeAll(&raw, input); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestProfileAnimatedGIFKeepsFramesDelayLoopAndDisposal(t *testing.T) {
	main, preview, err := encodeAnimatedAvatar(profileAnimatedFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(bytes.NewReader(main))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Image) != 3 || decoded.Config.Width != 512 || decoded.Config.Height != 512 || decoded.LoopCount != 2 {
		t.Fatal("actual canonical animation lost")
	}
	for i, delay := range []int{7, 13, 19} {
		if decoded.Delay[i] != delay {
			t.Fatal("frame delay changed")
		}
	}
	blue := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return b > 50000 && r < 10000 }
	red := func(c color.Color) bool { r, _, b, _ := c.RGBA(); return r > 50000 && b < 10000 }
	if !red(decoded.Image[0].At(32, 32)) || !blue(decoded.Image[1].At(32, 32)) || !red(decoded.Image[2].At(32, 32)) || !blue(decoded.Image[2].At(480, 480)) {
		t.Fatal("frame composition or restore-previous disposal changed")
	}
	poster, err := jpeg.Decode(bytes.NewReader(preview))
	if err != nil || poster.Bounds().Dx() != 128 || poster.Bounds().Dy() != 128 {
		t.Fatal("independent static preview missing")
	}
}

func TestProfileAnimatedGIFRejectsAllocationAndMalformedBudgets(t *testing.T) {
	input := profileAnimatedFixture(t)
	large := append([]byte(nil), input...)
	binary.LittleEndian.PutUint16(large[6:8], 65535)
	binary.LittleEndian.PutUint16(large[8:10], 65535)
	if inspectAvatarGIF(large) == nil {
		t.Fatal("oversized canvas was decoded")
	}
	for _, bad := range [][]byte{input[:10], input[:len(input)-1], append(append([]byte(nil), input...), 0)} {
		if inspectAvatarGIF(bad) == nil {
			t.Fatal("malformed GIF accepted")
		}
	}
	palette := color.Palette{color.Black, color.White}
	frames := &gif.GIF{Config: image.Config{ColorModel: palette, Width: 1, Height: 1}}
	for i := 0; i <= avatarGIFFrameLimit; i++ {
		frames.Image = append(frames.Image, image.NewPaletted(image.Rect(0, 0, 1, 1), palette))
		frames.Delay = append(frames.Delay, 1)
	}
	var raw bytes.Buffer
	if gif.EncodeAll(&raw, frames) != nil || inspectAvatarGIF(raw.Bytes()) == nil {
		t.Fatal("too many frames bypassed pre-decode limit")
	}
}

func TestProfilePGOwnFieldsAndContactPrivacy(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	identity := f.nodes["identity"]
	me := deletionBody(t, uiRegressionCall(identity, "GET", "/api/me", f.token, nil), 200)
	registered, err := time.Parse(time.RFC3339Nano, me["createdAt"].(string))
	if err != nil || registered.IsZero() || me["id"] != f.user.ID {
		t.Fatal("actual immutable account identity missing")
	}
	bindings := me["contactBindings"].(map[string]any)
	if bindings["email"].(map[string]any)["state"] != "unbound" {
		t.Fatal("unbound authority missing")
	}
	sealed, err := identity.seal("verified@example.invalid", "profile-contact:"+identity.Config.DeploymentID+":"+f.user.ID+":email")
	if err != nil {
		t.Fatal(err)
	}
	deletionExec(t, f.ctx, identity, `INSERT INTO auth.contact_bindings(user_id,kind,value_ciphertext,verified_at) VALUES($1,'email',$2,now())`, f.user.ID, sealed)
	me = deletionBody(t, uiRegressionCall(identity, "GET", "/api/me", f.token, nil), 200)
	bindings = me["contactBindings"].(map[string]any)
	if bindings["email"].(map[string]any)["value"] != "verified@example.invalid" {
		t.Fatal("real encrypted binding did not project")
	}
	deletionBody(t, uiRegressionCall(identity, "GET", "/api/users/"+f.user.ID+"/profile", token, nil), 403)
	uiRegressionFriend(t, f, peer, token)
	other := deletionBody(t, uiRegressionCall(identity, "GET", "/api/users/"+f.user.ID+"/profile", token, nil), 200)
	data, _ := json.Marshal(other["contactBindings"])
	if strings.Contains(string(data), "verified@example") || strings.Contains(string(data), "unbound") || strings.Count(string(data), "hidden") != 2 {
		t.Fatal("other viewer learned binding presence/value")
	}
	signature := strings.Repeat("好", 200)
	changed := deletionBody(t, uiRegressionCall(f.nodes["directory"], "PATCH", "/api/me", f.token, map[string]string{"signature": signature}), 200)
	if changed["signature"] != signature {
		t.Fatal("signature-only update lost")
	}
	deletionBody(t, uiRegressionCall(f.nodes["directory"], "PATCH", "/api/me", f.token, map[string]string{"signature": signature + "好"}), 400)
	me = deletionBody(t, uiRegressionCall(identity, "GET", "/api/me", f.token, nil), 200)
	if me["signature"] != signature || me["createdAt"] != registered.UTC().Format(time.RFC3339Nano) {
		t.Fatal("invalid update changed stored signature or registration")
	}
	deletionBody(t, uiRegressionCall(f.nodes["directory"], "PUT", "/api/blocks/"+peer.ID, f.token, map[string]any{}), 200)
	deletionBody(t, uiRegressionCall(identity, "GET", "/api/users/"+f.user.ID+"/profile", token, nil), 403)
}
