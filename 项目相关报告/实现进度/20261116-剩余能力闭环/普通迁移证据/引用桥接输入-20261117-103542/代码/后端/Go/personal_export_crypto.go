package jobsim

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// A complete first pass authenticates every frame, the final frame and the
// immutable artifact digest before a single ZIP byte reaches the client.
// The caller keeps both the source fence and artifact fence through both passes.
func authenticatedPersonalZIP(ctx context.Context, f *os.File, key, expectedDigest []byte, expectedSize int64, dst io.Writer, check func() error) error {
	if len(key) != 32 || len(expectedDigest) != sha256.Size || expectedSize < 37 || expectedSize > 1<<30 {
		return fail(409, "EXPORT_INTEGRITY_FAILED", "Export integrity metadata unavailable")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	count := &personalExportCountWriter{Writer: h}
	if err := authenticatePersonalFrames(ctx, io.TeeReader(io.LimitReader(f, expectedSize+1), count), io.Discard, key, check); err != nil {
		return fail(409, "EXPORT_INTEGRITY_FAILED", "Export artifact did not pass complete authentication")
	}
	if count.Count != expectedSize || !exportEqualDigest(h.Sum(nil), expectedDigest) {
		return fail(409, "EXPORT_INTEGRITY_FAILED", "Export artifact digest differs")
	}
	if err := check(); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return authenticatePersonalFrames(ctx, io.LimitReader(f, expectedSize+1), dst, key, check)
}

type personalExportCountWriter struct {
	io.Writer
	Count int64
}

func (w *personalExportCountWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.Count += int64(n)
	return n, err
}

func authenticatePersonalFrames(ctx context.Context, src io.Reader, dst io.Writer, key []byte, check func() error) error {
	if len(key) != 32 {
		return fmt.Errorf("invalid personal export key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	header := make([]byte, 16)
	if _, err = io.ReadFull(src, header); err != nil || string(header[:8]) != archiveMagic {
		return fmt.Errorf("invalid personal export header")
	}
	for index := uint32(0); index < ^uint32(0); index++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = check(); err != nil {
			return err
		}
		frame := make([]byte, 5)
		if _, err = io.ReadFull(src, frame); err != nil {
			return fmt.Errorf("personal export missing authenticated final frame")
		}
		size := binary.BigEndian.Uint32(frame[:4])
		if size > 65536 || frame[4] > 1 || frame[4] == 1 && size != 0 {
			return fmt.Errorf("invalid personal export frame")
		}
		sealed := make([]byte, int(size)+gcm.Overhead())
		if _, err = io.ReadFull(src, sealed); err != nil {
			return err
		}
		nonce := make([]byte, gcm.NonceSize())
		copy(nonce, header[8:])
		binary.BigEndian.PutUint32(nonce[8:], index)
		aad := append(append([]byte(archiveMagic), frame...), nonce[8:]...)
		plain, err := gcm.Open(nil, nonce, sealed, aad)
		if err != nil {
			return fmt.Errorf("personal export authentication failed")
		}
		if frame[4] == 1 {
			var extra [1]byte
			n, err := src.Read(extra[:])
			if n != 0 || err != io.EOF {
				return fmt.Errorf("personal export contains unauthenticated trailing bytes")
			}
			return nil
		}
		if _, err = dst.Write(plain); err != nil {
			return err
		}
	}
	return fmt.Errorf("personal export frame capacity exceeded")
}
