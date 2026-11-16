package jobsim

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
	"regexp"
	"strings"
	"unicode/utf8"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

func normalizeUsername(v string) (string, error) {
	if !usernamePattern.MatchString(v) {
		return "", fail(400, "INVALID_USERNAME", "Username requires 3–32 ASCII letters, digits or underscores")
	}
	return strings.ToLower(v), nil
}
func normalizePassword(v string) (string, error) {
	v = norm.NFC.String(v)
	n := utf8.RuneCountInString(v)
	if !utf8.ValidString(v) || n < 15 || n > 128 || len(v) > 512 {
		return "", fail(400, "INVALID_PASSWORD", "Password requires 15–128 Unicode characters and at most 512 UTF-8 bytes")
	}
	weak := map[string]bool{"passwordpassword": true, "123456789012345": true, "1234567890123456": true, "qwertyuiopasdfgh": true, "letmeinletmeinletmein": true}
	if weak[strings.ToLower(v)] {
		return "", fail(400, "WEAK_PASSWORD", "Choose a less common password")
	}
	return v, nil
}
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func tokenDigest(s string) []byte { x := sha256.Sum256([]byte(s)); return x[:] }
func keyedDigest(key []byte, purpose string, v []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(purpose))
	m.Write([]byte{0})
	m.Write(v)
	return m.Sum(nil)
}
func hashPassword(v string) string {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	hash := argon2.IDKey([]byte(v), salt, 3, 64*1024, 1, 32)
	return "$argon2id$v=19$m=65536,t=3,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
}
func verifyPassword(encoded, v string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 6 || p[1] != "argon2id" || p[2] != "v=19" || p[3] != "m=65536,t=3,p=1" {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(p[4])
	if e != nil || len(salt) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(p[5])
	if e != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(norm.NFC.String(v)), salt, 3, 64*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func stableID(key []byte, context string) string {
	b := keyedDigest(key, "stable-object-id", []byte(context))[:16]
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
