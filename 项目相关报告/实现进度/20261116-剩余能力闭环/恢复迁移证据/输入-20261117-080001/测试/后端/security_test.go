package jobsim

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestUsernameAndPasswordBoundaries(t *testing.T) {
	for _, v := range []string{"a", "name with space", " 中文名", "abc ", "ümlaut"} {
		if _, e := normalizeUsername(v); e == nil {
			t.Fatalf("accepted invalid username %q", v)
		}
	}
	if got, e := normalizeUsername("Jobs_IM"); e != nil || got != "jobs_im" {
		t.Fatal("canonical username")
	}
	for _, v := range []string{"12345678901234", strings.Repeat("a", 129), "1234567890123456"} {
		if _, e := normalizePassword(v); e == nil {
			t.Fatal("accepted invalid or weak password")
		}
	}
	original := "  Different safe password 42  "
	normalized, e := normalizePassword(original)
	if e != nil || normalized != original {
		t.Fatal("password whitespace changed")
	}
	decomposed := "e\u0301" + strings.Repeat("x", 14)
	got, e := normalizePassword(decomposed)
	if e != nil || got != "é"+strings.Repeat("x", 14) {
		t.Fatal("NFC normalization mismatch")
	}
}

func TestMalformedPathsCannotBecomeRestoreFacts(t *testing.T) {
	id := newID()
	valid := map[string]string{"userId": id, "generation": "2", "securityVersion": "3", "banUntil": "", "expiresAt": time.Now().UTC().Format(time.RFC3339Nano), "state": "tombstoned"}
	for field, value := range valid {
		if e := validateJournalField(field, value); e != nil {
			t.Fatalf("valid minimal field rejected: %s", field)
		}
	}
	for _, v := range []struct {
		field string
		value any
	}{{"deviceId", "not-a-uuid"}, {"userId", "00000000-0000-0000-0000-00000000000g"}, {"generation", "0"}, {"generation", "01"}, {"generation", "9223372036854775808"}, {"securityVersion", float64(2)}, {"expiresAt", "tomorrow"}, {"state", "anything"}} {
		if e := validateJournalField(v.field, v.value); e == nil {
			t.Fatalf("unsafe restore field accepted: %s", v.field)
		}
	}
}

func TestAuthenticatedBackupRejectsTruncationReorderAndTrailingData(t *testing.T) {
	key := bytes.Repeat([]byte{19}, 32)
	plain := bytes.Repeat([]byte("private data fixture\n"), 9000)
	var encrypted bytes.Buffer
	writer, e := newChunkWriter(&encrypted, key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = writer.Write(plain); e != nil {
		t.Fatal(e)
	}
	if e = writer.Close(); e != nil {
		t.Fatal(e)
	}
	archive := encrypted.Bytes()
	var decoded bytes.Buffer
	if e = decryptArchive(bytes.NewReader(archive), &decoded, key); e != nil || !bytes.Equal(decoded.Bytes(), plain) {
		t.Fatal("valid multi-frame archive failed")
	}
	corrupt := append([]byte(nil), archive...)
	corrupt[40] ^= 1
	cases := [][]byte{archive[:15], archive[:len(archive)-1], archive[:len(archive)-21], append(append([]byte(nil), archive...), 1), corrupt}
	frame := 5 + 65536 + 16
	reordered := append([]byte(nil), archive...)
	copy(reordered[16:16+frame], archive[16+frame:16+frame*2])
	copy(reordered[16+frame:16+frame*2], archive[16:16+frame])
	cases = append(cases, reordered)
	for _, damaged := range cases {
		var out bytes.Buffer
		if e = decryptArchive(bytes.NewReader(damaged), &out, key); e == nil {
			t.Fatal("damaged authenticated archive accepted")
		}
	}
	var wrong bytes.Buffer
	if e = decryptArchive(bytes.NewReader(archive), &wrong, bytes.Repeat([]byte{20}, 32)); e == nil {
		t.Fatal("wrong recovery key accepted")
	}
}
func TestArgonAndCredentialSealing(t *testing.T) {
	password := "Correct horse long phrase 924!"
	encoded := hashPassword(password)
	if !verifyPassword(encoded, password) || verifyPassword(encoded, password+" ") {
		t.Fatal("password verification mismatch")
	}
	if verifyPassword("$argon2id$v=19$m=999999999,t=3,p=1$a$b", password) {
		t.Fatal("unbounded attacker parameters accepted")
	}
	app := &App{Config: Config{Key: bytes.Repeat([]byte{42}, 32)}}
	sealed, e := app.seal("secret credential", "operation-a")
	if e != nil {
		t.Fatal(e)
	}
	if got, e := app.unseal(sealed, "operation-a"); e != nil || got != "secret credential" {
		t.Fatal("credential delivery failed")
	}
	if _, e = app.unseal(sealed, "operation-b"); e == nil {
		t.Fatal("credential replayed under another request")
	}
	sealed[len(sealed)-1] ^= 1
	if _, e = app.unseal(sealed, "operation-a"); e == nil {
		t.Fatal("tampered credential accepted")
	}
}

func TestRFC6238CounterAndPayloadBinding(t *testing.T) {
	secret := []byte("12345678901234567890")
	if totpCode(secret, 1) != "287082" {
		t.Fatal("RFC TOTP dynamic truncation mismatch")
	}
	first, e := canonicalAdminPayload([]byte(`{"enabled":true,"expectedVersion":"1","requestId":"original","stepUpId":"proof"}`))
	if e != nil {
		t.Fatal(e)
	}
	second, e := canonicalAdminPayload([]byte(`{"expectedVersion":"1","enabled":true}`))
	if e != nil || !bytes.Equal(first, second) {
		t.Fatal("equivalent operation payloads differ")
	}
	changed, _ := canonicalAdminPayload([]byte(`{"expectedVersion":"2","enabled":true}`))
	if bytes.Equal(first, changed) {
		t.Fatal("version not bound into step-up digest")
	}
}
