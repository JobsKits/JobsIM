package jobsim

import (
	"fmt"
	"io"
	"strings"
)

const (
	webmEBML     = 0x1a45dfa3
	webmSegment  = 0x18538067
	webmTracksID = 0x1654ae6b
	webmEntry    = 0xae
	webmCluster  = 0x1f43b675
	webmVoid     = 0xec
	webmCRC      = 0xbf

	webmElementLimit  = 20000
	webmDepthLimit    = 8
	webmTrackLimit    = 64
	webmReadLimit     = 1 << 20
	webmMetadataLimit = 4 << 20
	webmFieldLimit    = 16 << 20
)

type webmElement struct {
	id        uint64
	data, end int64
	unknown   bool
}

type webmTrackParser struct {
	r                      io.ReaderAt
	size                   int64
	elements, read         int
	maxID, maxSize, tracks int
	numbers, uids          map[uint64]bool
}

// Read only element headers and bounded track metadata. Encoded packets remain
// opaque; their bytes are never searched for apparent track IDs or codecs.
func webmTracks(r io.ReaderAt, size int64) (audio, video bool, err error) {
	p := &webmTrackParser{r: r, size: size, maxID: 4, maxSize: 8, numbers: map[uint64]bool{}, uids: map[uint64]bool{}}
	if size < 5 {
		return false, false, fmt.Errorf("truncated EBML header")
	}
	var last [1]byte
	if err = p.readAt(last[:], size-1, size); err != nil {
		return false, false, err
	}
	header, err := p.element(0, size)
	if err != nil || header.id != webmEBML || header.unknown || header.end-header.data > 4096 {
		return false, false, fmt.Errorf("invalid EBML header")
	}
	if err = p.header(header); err != nil {
		return false, false, err
	}
	segmentSeen := false
	for pos := header.end; pos < size; {
		e, eerr := p.element(pos, size)
		if eerr != nil {
			return false, false, eerr
		}
		if e.id == webmVoid || e.id == webmCRC {
			if err = p.opaque(e); err != nil {
				return false, false, err
			}
		} else if e.id == webmSegment && !segmentSeen {
			segmentSeen = true
			audio, video, err = p.segment(e)
			if err != nil {
				return false, false, err
			}
		} else {
			return false, false, fmt.Errorf("unexpected root element")
		}
		pos = e.end
	}
	if !segmentSeen || p.tracks == 0 || !audio && !video {
		return false, false, fmt.Errorf("valid Tracks absent")
	}
	return audio, video, nil
}

func (p *webmTrackParser) readAt(b []byte, pos, end int64) error {
	if pos < 0 || end > p.size || pos > end || int64(len(b)) > end-pos || p.read > webmReadLimit-len(b) {
		return fmt.Errorf("WebM read bounds")
	}
	p.read += len(b)
	n, err := p.r.ReadAt(b, pos)
	if err != nil || n != len(b) {
		return fmt.Errorf("truncated WebM field")
	}
	return nil
}

func (p *webmTrackParser) vint(pos, end int64, limit int, id bool) (uint64, int, bool, error) {
	var b [8]byte
	if err := p.readAt(b[:1], pos, end); err != nil {
		return 0, 0, false, err
	}
	width, marker := 1, byte(0x80)
	for marker != 0 && b[0]&marker == 0 {
		width++
		marker >>= 1
	}
	if marker == 0 || width > limit {
		return 0, 0, false, fmt.Errorf("invalid VINT width")
	}
	if width > 1 {
		if err := p.readAt(b[1:width], pos+1, end); err != nil {
			return 0, 0, false, err
		}
	}
	value := uint64(b[0] & (marker - 1))
	for _, octet := range b[1:width] {
		value = value<<8 | uint64(octet)
	}
	maximum := uint64(1)<<(7*width) - 1
	if id {
		// IDs retain their marker and must use the shortest non-reserved form.
		// RFC 9559 corrects RFC 8794: the deployed one-byte ID 0x80 is valid.
		if value == 0 && width != 1 || value == maximum || width > 1 && value < uint64(1)<<(7*(width-1))-1 {
			return 0, 0, false, fmt.Errorf("invalid element ID")
		}
		value |= uint64(marker) << (8 * (width - 1))
		return value, width, false, nil
	}
	return value, width, value == maximum, nil
}

func (p *webmTrackParser) element(pos, end int64) (webmElement, error) {
	var e webmElement
	p.elements++
	if p.elements > webmElementLimit {
		return e, fmt.Errorf("WebM element limit")
	}
	id, width, _, err := p.vint(pos, end, p.maxID, true)
	if err != nil {
		return e, err
	}
	length, sizeWidth, unknown, err := p.vint(pos+int64(width), end, p.maxSize, false)
	if err != nil {
		return e, err
	}
	e.id, e.data, e.unknown = id, pos+int64(width+sizeWidth), unknown
	if unknown {
		if id != webmSegment && id != webmCluster {
			return e, fmt.Errorf("unknown-sized non-streaming element")
		}
		e.end = end
	} else {
		if e.data > end || length > uint64(end-e.data) {
			return e, fmt.Errorf("element exceeds parent")
		}
		e.end = e.data + int64(length)
	}
	return e, nil
}

func (p *webmTrackParser) uint(e webmElement) (uint64, error) {
	length := e.end - e.data
	if e.unknown || length < 1 || length > 8 {
		return 0, fmt.Errorf("invalid unsigned track field")
	}
	var b [8]byte
	if err := p.readAt(b[:length], e.data, e.end); err != nil {
		return 0, err
	}
	var value uint64
	for _, octet := range b[:length] {
		value = value<<8 | uint64(octet)
	}
	return value, nil
}

func (p *webmTrackParser) text(e webmElement, limit int64) (string, error) {
	length := e.end - e.data
	if e.unknown || length < 1 || length > limit || limit > 64 {
		return "", fmt.Errorf("invalid track string length")
	}
	var b [64]byte
	if err := p.readAt(b[:length], e.data, e.end); err != nil {
		return "", err
	}
	value := strings.TrimRight(string(b[:length]), "\x00")
	for _, octet := range []byte(value) {
		if octet < 0x20 || octet > 0x7e {
			return "", fmt.Errorf("invalid track string")
		}
	}
	return value, nil
}

func (p *webmTrackParser) header(header webmElement) error {
	seen := map[uint64]bool{}
	docType := ""
	maxSize := p.maxSize
	for pos := header.data; pos < header.end; {
		e, err := p.element(pos, header.end)
		if err != nil {
			return err
		}
		if seen[e.id] && e.id != webmVoid {
			return fmt.Errorf("duplicate EBML header field")
		}
		seen[e.id] = true
		switch e.id {
		case 0x4282:
			docType, err = p.text(e, 16)
		case 0x4286, 0x42f7, 0x42f2, 0x42f3, 0x4287, 0x4285:
			var value uint64
			value, err = p.uint(e)
			if err == nil && (value == 0 || e.id == 0x42f7 && value > 1 || e.id == 0x42f2 && value != 4 || e.id == 0x42f3 && value > 8 || e.id == 0x4285 && value > 4) {
				err = fmt.Errorf("unsupported EBML header limits")
			}
			if err == nil && e.id == 0x42f3 {
				maxSize = int(value)
			}
		default:
			err = p.opaque(e)
		}
		if err != nil {
			return err
		}
		pos = e.end
	}
	if docType != "webm" {
		return fmt.Errorf("WebM DocType required")
	}
	p.maxSize = maxSize
	return nil
}

func (p *webmTrackParser) segment(segment webmElement) (audio, video bool, err error) {
	tracksSeen, clusterSeen := false, false
	for pos := segment.data; pos < segment.end; {
		e, eerr := p.element(pos, segment.end)
		if eerr != nil {
			return false, false, eerr
		}
		switch e.id {
		case webmTracksID:
			if tracksSeen || clusterSeen || e.end-e.data > webmMetadataLimit {
				return false, false, fmt.Errorf("duplicate, late or oversized Tracks")
			}
			tracksSeen = true
			audio, video, err = p.trackEntries(e)
		case webmCluster:
			if !tracksSeen {
				return false, false, fmt.Errorf("Tracks must precede Cluster")
			}
			clusterSeen = true
			e.end, err = p.cluster(e, 1)
		case webmEBML, webmSegment, webmEntry:
			return false, false, fmt.Errorf("unexpected Segment child")
		default:
			err = p.metadata(e, 1)
		}
		if err != nil {
			return false, false, err
		}
		pos = e.end
	}
	if !tracksSeen {
		return false, false, fmt.Errorf("Tracks absent")
	}
	return audio, video, nil
}

func (p *webmTrackParser) trackEntries(tracks webmElement) (audio, video bool, err error) {
	for pos := tracks.data; pos < tracks.end; {
		e, eerr := p.element(pos, tracks.end)
		if eerr != nil {
			return false, false, eerr
		}
		if e.id == webmEntry {
			p.tracks++
			if p.tracks > webmTrackLimit || e.end-e.data > 1<<20 {
				return false, false, fmt.Errorf("track count or size limit")
			}
			var kind uint64
			kind, err = p.track(e)
			audio, video = audio || kind == 2, video || kind == 1
		} else if e.id == webmVoid || e.id == webmCRC {
			err = p.opaque(e)
		} else {
			return false, false, fmt.Errorf("unexpected Tracks child")
		}
		if err != nil {
			return false, false, err
		}
		pos = e.end
	}
	return audio, video, nil
}

func (p *webmTrackParser) track(entry webmElement) (uint64, error) {
	fields := map[uint64]bool{}
	var number, uid, kind uint64
	codec := ""
	for pos := entry.data; pos < entry.end; {
		e, err := p.element(pos, entry.end)
		if err != nil {
			return 0, err
		}
		switch e.id {
		case 0xd7, 0x73c5, 0x83, 0x86:
			if fields[e.id] {
				return 0, fmt.Errorf("duplicate track identity field")
			}
			fields[e.id] = true
			switch e.id {
			case 0xd7:
				number, err = p.uint(e)
			case 0x73c5:
				uid, err = p.uint(e)
			case 0x83:
				kind, err = p.uint(e)
			case 0x86:
				codec, err = p.text(e, 64)
			}
		case webmEBML, webmSegment, webmTracksID, webmEntry, webmCluster:
			return 0, fmt.Errorf("misplaced track structure")
		default:
			err = p.metadata(e, 3)
		}
		if err != nil {
			return 0, err
		}
		pos = e.end
	}
	if number == 0 || uid == 0 || p.numbers[number] || p.uids[uid] || kind != 1 && kind != 2 {
		return 0, fmt.Errorf("invalid or duplicate track identity")
	}
	if kind == 2 && codec != "A_OPUS" && codec != "A_VORBIS" || kind == 1 && (!strings.HasPrefix(codec, "V_") || len(codec) <= 2) {
		return 0, fmt.Errorf("track type and codec disagree")
	}
	p.numbers[number], p.uids[uid] = true, true
	return kind, nil
}

func (p *webmTrackParser) opaque(e webmElement) error {
	if e.unknown || e.end-e.data > webmFieldLimit || e.id == webmCRC && e.end-e.data != 4 {
		return fmt.Errorf("opaque field size limit")
	}
	return nil
}

func webmTopLevel(id uint64) bool {
	switch id {
	case webmEBML, webmSegment, webmCluster, webmTracksID, 0x114d9b74, 0x1549a966, 0x1c53bb6b, 0x1941a469, 0x1043a770, 0x1254c367:
		return true
	}
	return false
}

func (p *webmTrackParser) cluster(cluster webmElement, depth int) (int64, error) {
	for pos := cluster.data; pos < cluster.end; {
		e, err := p.element(pos, cluster.end)
		if err != nil {
			return 0, err
		}
		if webmTopLevel(e.id) {
			if cluster.unknown {
				return pos, nil
			}
			return 0, fmt.Errorf("top-level element inside sized Cluster")
		}
		if err = p.metadata(e, depth+1); err != nil {
			return 0, err
		}
		pos = e.end
	}
	return cluster.end, nil
}

func (p *webmTrackParser) metadata(e webmElement, depth int) error {
	if depth > webmDepthLimit || e.unknown {
		return fmt.Errorf("metadata depth or unknown-size limit")
	}
	master := false
	switch e.id {
	case 0x114d9b74, 0x4dbb, 0x1549a966, 0x6924, 0xe0, 0xe1, 0x55b0, 0x55d0, 0x7670,
		0x6d80, 0x6240, 0x5034, 0x5035, 0x47e7, 0xa0, 0x75a1, 0xa6,
		0x1c53bb6b, 0xbb, 0xb7, 0x1941a469, 0x61a7, 0x1043a770, 0x45b9, 0xb6, 0x80,
		0x1254c367, 0x7373, 0x63c0, 0x67c8:
		master = true
	case webmEBML, webmSegment, webmTracksID, webmEntry, webmCluster:
		return fmt.Errorf("misplaced structural element")
	}
	if !master {
		if e.id == 0xa3 || e.id == 0xa1 {
			// Encoded packets require no allocation or decoding. Their declared
			// extent was already checked against the actual opened file size.
			return nil
		}
		return p.opaque(e)
	}
	if e.id != 0xa0 && e.end-e.data > webmMetadataLimit {
		return fmt.Errorf("metadata length limit")
	}
	for pos := e.data; pos < e.end; {
		child, err := p.element(pos, e.end)
		if err != nil {
			return err
		}
		if err = p.metadata(child, depth+1); err != nil {
			return err
		}
		pos = child.end
	}
	return nil
}
