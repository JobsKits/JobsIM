package jobsim

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// Synthetic container metadata, not encoded audio or a browser recording.
func webmTestSize(value uint64) []byte {
	for width := 1; width <= 8; width++ {
		if value >= uint64(1)<<(7*width)-1 {
			continue
		}
		out := make([]byte, width)
		for i := width - 1; i >= 0; i-- {
			out[i] = byte(value)
			value >>= 8
		}
		out[0] |= 1 << (8 - width)
		return out
	}
	panic("synthetic WebM size out of range")
}

func webmTestID(id uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], id)
	return bytes.TrimLeft(b[:], "\x00")
}

func webmTestElement(id uint32, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	out := append([]byte{}, webmTestID(id)...)
	out = append(out, webmTestSize(uint64(len(body)))...)
	return append(out, body...)
}

func webmTestUnknown(id uint32, payload ...[]byte) []byte {
	out := append([]byte{}, webmTestID(id)...)
	out = append(out, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)
	return append(out, bytes.Join(payload, nil)...)
}

func webmTestUint(id uint32, value uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], value)
	data := bytes.TrimLeft(b[:], "\x00")
	if len(data) == 0 {
		data = []byte{0}
	}
	return webmTestElement(id, data)
}

func webmTestHeader() []byte {
	return webmTestElement(webmEBML, webmTestElement(0x4282, []byte("webm")), webmTestUint(0x4286, 1), webmTestUint(0x42f7, 1), webmTestUint(0x42f2, 4), webmTestUint(0x42f3, 8), webmTestUint(0x4287, 4), webmTestUint(0x4285, 2))
}

func webmTestTrack(number, kind uint64, codec string, extra ...[]byte) []byte {
	fields := [][]byte{webmTestUint(0xd7, number), webmTestUint(0x73c5, number+100), webmTestUint(0x83, kind), webmTestElement(0x86, []byte(codec))}
	return webmTestElement(webmEntry, append(fields, extra...)...)
}

func webmTestFile(unknown bool, children ...[]byte) []byte {
	segment := webmTestElement(webmSegment, children...)
	if unknown {
		segment = webmTestUnknown(webmSegment, children...)
	}
	return append(webmTestHeader(), segment...)
}

func TestWebMClassifiesActualTracksInsteadOfSniffOrName(t *testing.T) {
	audio := webmTestTrack(1, 2, "A_OPUS", webmTestElement(0xe1, webmTestUint(0x9f, 1)))
	video := webmTestTrack(2, 1, "V_VP9", webmTestElement(0xe0, webmTestUint(0xb0, 16), webmTestUint(0xba, 16)))
	cluster := webmTestElement(webmCluster, webmTestUint(0xe7, 0), webmTestElement(0xa3, []byte{0x81, 0, 0, 0x80, 0x11}))
	for _, tc := range []struct {
		name, sniff, want string
		data              []byte
	}{
		{"opus-browser-sniff", "video/webm", "audio/webm", webmTestFile(false, webmTestElement(webmTracksID, audio), cluster)},
		{"vorbis-streaming-segment", "video/webm", "audio/webm", webmTestFile(true, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_VORBIS")), cluster)},
		{"video-and-audio", "audio/webm", "video/webm", webmTestFile(false, webmTestElement(webmTracksID, audio, video), cluster)},
		{"video-only", "audio/webm", "video/webm", webmTestFile(true, webmTestElement(webmTracksID, video), cluster)},
		{"binary-private-prefix-is-opaque", "image/png", "audio/webm", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS", webmTestElement(0x63a2, webmTestTrack(2, 1, "V_VP8")))), cluster)},
		{"unknown-clusters-and-trailing-cues", "video/webm", "audio/webm", webmTestFile(true, webmTestElement(webmTracksID, audio), webmTestUnknown(webmCluster, webmTestUint(0xe7, 0), webmTestElement(0xa3, []byte{0x81, 0, 0, 0x80})), webmTestUnknown(webmCluster, webmTestUint(0xe7, 20)), webmTestElement(0x1c53bb6b))},
		{"rfc9559-chapter-display-id", "video/webm", "audio/webm", webmTestFile(false, webmTestElement(webmTracksID, audio), cluster, webmTestElement(0x1043a770, webmTestElement(0x45b9, webmTestElement(0xb6, webmTestElement(0x80, webmTestElement(0x85, []byte("Caption")))))))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "claimed-audio.mp3")
			if err := os.WriteFile(file, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := classifyStoredMedia(file, tc.sniff)
			if err != nil || got != tc.want {
				t.Fatalf("actual track classification got %q, %v; want %s", got, err, tc.want)
			}
		})
	}
	if got := http.DetectContentType(webmTestFile(true, webmTestElement(webmTracksID, audio), cluster)); got != "video/webm" {
		t.Fatalf("synthetic fixture did not reproduce standard WebM sniff: %s", got)
	}
}

func TestWebMRejectsSpoofedAndMalformedTrackStructures(t *testing.T) {
	audio := webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS"))
	cluster := webmTestElement(webmCluster, webmTestUint(0xe7, 0))
	missingCodec := webmTestElement(webmEntry, webmTestUint(0xd7, 1), webmTestUint(0x73c5, 101), webmTestUint(0x83, 2), webmTestElement(0x63a2, []byte("A_OPUS")))
	missingType := webmTestElement(webmEntry, webmTestUint(0xd7, 1), webmTestUint(0x73c5, 101), webmTestElement(0x86, []byte("A_OPUS")))
	valid := webmTestFile(false, audio, cluster)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"audio-type-with-video-codec", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "V_VP8")))},
		{"video-type-with-audio-codec", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 1, "A_OPUS")))},
		{"unsupported-audio-codec", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_FLAC")))},
		{"missing-codec-binary-decoy", webmTestFile(false, webmTestElement(webmTracksID, missingCodec))},
		{"missing-type", webmTestFile(false, webmTestElement(webmTracksID, missingType))},
		{"empty-tracks", webmTestFile(false, webmTestElement(webmTracksID))},
		{"no-tracks", webmTestFile(false, cluster)},
		{"tracks-after-cluster", webmTestFile(false, audio, cluster, audio)},
		{"duplicate-tracks", webmTestFile(false, audio, audio)},
		{"duplicate-number-and-uid", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS"), webmTestTrack(1, 1, "V_VP9")))},
		{"duplicate-type", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS", webmTestUint(0x83, 1))))},
		{"duplicate-codec", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS", webmTestElement(0x86, []byte("V_VP9")))))},
		{"oversized-codec", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, string(bytes.Repeat([]byte{'A'}, 65)))))},
		{"embedded-string-nul", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS\x00V_VP9")))},
		{"misplaced-entry", webmTestFile(false, webmTestTrack(1, 2, "A_OPUS"))},
		{"unknown-tracks-size", webmTestFile(true, webmTestUnknown(webmTracksID, webmTestTrack(1, 2, "A_OPUS")))},
		{"unknown-entry-size", webmTestFile(true, webmTestElement(webmTracksID, webmTestUnknown(webmEntry)))},
		{"unknown-info-size", webmTestFile(true, audio, webmTestUnknown(0x1549a966))},
		{"unknown-block-size", webmTestFile(true, audio, webmTestUnknown(webmCluster, webmTestUnknown(0xa3)))},
		{"truncated-payload", valid[:len(valid)-1]},
		{"zero-vint", webmTestFile(false, audio, []byte{0})},
		{"reserved-multibyte-zero-id", webmTestFile(false, audio, []byte{0x40, 0, 0x80})},
		{"reserved-one-id", webmTestFile(false, audio, []byte{0xff, 0x80})},
		{"nonminimal-id", webmTestFile(false, audio, []byte{0x40, 0x01, 0x80})},
		{"oversized-id", webmTestFile(false, audio, []byte{0x08, 1, 2, 3, 4, 0x80})},
		{"truncated-size-vint", webmTestFile(false, audio, []byte{0xec, 0x01, 0, 0})},
		{"maximum-size-exceeds-file", webmTestFile(false, audio, []byte{0xec, 0x01, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe})},
		{"second-segment", append(valid, webmTestElement(webmSegment, audio)...)},
		{"wrong-doctype", append(webmTestElement(webmEBML, webmTestElement(0x4282, []byte("matroska"))), webmTestElement(webmSegment, audio)...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := webmTracks(bytes.NewReader(tc.data), int64(len(tc.data))); err == nil {
				t.Fatal("untrusted WebM structure was accepted")
			}
		})
	}
}

type webmTestGuardReader struct {
	data       []byte
	reads, max int
}

func (r *webmTestGuardReader) ReadAt(b []byte, pos int64) (int, error) {
	r.reads += len(b)
	if len(b) > r.max {
		r.max = len(b)
	}
	if len(b) > 64 || pos < 0 || pos > int64(len(r.data)) {
		return 0, io.ErrUnexpectedEOF
	}
	return bytes.NewReader(r.data).ReadAt(b, pos)
}

func TestWebMBoundsElementsDepthTracksAndReaderAt(t *testing.T) {
	audio := webmTestTrack(1, 2, "A_OPUS")
	valid := webmTestFile(true, webmTestElement(webmTracksID, audio), webmTestElement(webmCluster, webmTestElement(0xa3, bytes.Repeat([]byte{0x5a}, 1024))))
	guard := &webmTestGuardReader{data: valid}
	if a, v, err := webmTracks(guard, int64(len(valid))); err != nil || !a || v || guard.max > 64 || guard.reads > webmReadLimit {
		t.Fatal("bounded ReaderAt could not classify metadata without reading packet payloads")
	}
	if _, _, err := webmTracks(bytes.NewReader(valid[:len(valid)-1]), int64(len(valid))); err == nil {
		t.Fatal("ReaderAt short physical file was accepted despite advertised size")
	}
	var entries [][]byte
	for i := 1; i <= webmTrackLimit+1; i++ {
		entries = append(entries, webmTestTrack(uint64(i), 2, "A_OPUS"))
	}
	nested := webmTestUint(0x9f, 1)
	for i := 0; i <= webmDepthLimit; i++ {
		nested = webmTestElement(0xe1, nested)
	}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"track-count", webmTestFile(false, webmTestElement(webmTracksID, entries...))},
		{"nested-metadata", webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS", nested)))},
		{"element-count", webmTestFile(false, webmTestElement(webmTracksID, audio), bytes.Repeat(webmTestElement(webmVoid), webmElementLimit))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := webmTracks(bytes.NewReader(tc.data), int64(len(tc.data))); err == nil {
				t.Fatal("WebM resource bound did not reject oversized structure")
			}
		})
	}
}

func TestWebMRejectsMalformedContainerThroughStoredMediaEntry(t *testing.T) {
	data := webmTestFile(true, webmTestElement(webmTracksID, webmTestTrack(1, 2, "V_VP9")))
	file := filepath.Join(t.TempDir(), "claimed-audio.webm")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := classifyStoredMedia(file, "audio/webm"); err == nil {
		t.Fatal("client audio MIME bypassed actual invalid WebM classification")
	}
}

type webmTestVirtualReader struct {
	prefix     []byte
	size       int64
	reads, max int
}

func (r *webmTestVirtualReader) ReadAt(b []byte, pos int64) (int, error) {
	r.reads += len(b)
	if len(b) > r.max {
		r.max = len(b)
	}
	if len(b) > 64 || pos < 0 || pos > r.size || int64(len(b)) > r.size-pos {
		return 0, io.ErrUnexpectedEOF
	}
	clear(b)
	if pos < int64(len(r.prefix)) {
		copy(b, r.prefix[pos:])
	}
	return len(b), nil
}

func TestWebMSkipsLargeEncodedPacketsButBoundsMetadata(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   uint32
		want bool
	}{
		{"large-simple-block", 0xa3, true},
		{"large-metadata-field", webmVoid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := webmTestFile(true, webmTestElement(webmTracksID, webmTestTrack(1, 2, "A_OPUS")), webmTestUnknown(webmCluster))
			prefix = append(prefix, webmTestID(tc.id)...)
			prefix = append(prefix, webmTestSize(webmFieldLimit+1)...)
			r := &webmTestVirtualReader{prefix: prefix, size: int64(len(prefix)) + webmFieldLimit + 1}
			audio, video, err := webmTracks(r, r.size)
			if (err == nil) != tc.want || tc.want && (!audio || video) || r.max > 64 || r.reads > webmReadLimit {
				t.Fatal("large encoded packet and bounded metadata were not kept separate")
			}
		})
	}
}

func TestWebMPreservesBMFFTrackClassification(t *testing.T) {
	box := func(kind string, data []byte) []byte {
		out := make([]byte, 8+len(data))
		binary.BigEndian.PutUint32(out[:4], uint32(len(out)))
		copy(out[4:8], kind)
		copy(out[8:], data)
		return out
	}
	track := func(kind string) []byte {
		handler := append(make([]byte, 8), []byte(kind)...)
		return box("trak", box("mdia", box("hdlr", handler)))
	}
	ftyp := box("ftyp", []byte{'i', 's', 'o', 'm', 0, 0, 0, 0, 'i', 's', 'o', 'm'})
	for _, tc := range []struct {
		name, want string
		tracks     []byte
	}{
		{"audio", "audio/mp4", track("soun")},
		{"video", "video/mp4", track("vide")},
		{"mixed", "video/mp4", append(track("soun"), track("vide")...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := append(append([]byte{}, ftyp...), box("moov", tc.tracks)...)
			file := filepath.Join(t.TempDir(), "claimed-webm.webm")
			if err := os.WriteFile(file, data, 0600); err != nil {
				t.Fatal(err)
			}
			if got, err := classifyStoredMedia(file, "video/mp4"); err != nil || got != tc.want {
				t.Fatal("WebM entry changed existing BMFF audio/video classification")
			}
		})
	}
}
