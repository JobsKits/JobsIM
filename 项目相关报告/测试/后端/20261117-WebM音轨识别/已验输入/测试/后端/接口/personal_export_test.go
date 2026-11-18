package jobsim

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func personalExportTestArtifact(t *testing.T, key []byte) []byte {
	t.Helper()
	var encrypted bytes.Buffer
	writer, err := newChunkWriter(&encrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	out := zip.NewWriter(writer)
	body := bytes.Repeat([]byte("JobsIM ordinary export independent owned text.\n"), 5000)
	if err = writePersonalZIPEntry(out, "owned.txt", bytes.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	// Store incompressible deterministic bytes to cover several AEAD frames.
	data := make([]byte, 180000)
	for offset := 0; offset < len(data); offset += 32 {
		sum := sha256.Sum256([]byte(strings.Repeat("x", offset/32+1)))
		copy(data[offset:], sum[:])
	}
	entry, err := out.CreateHeader(&zip.FileHeader{Name: "sample.bin", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return encrypted.Bytes()
}

func TestPersonalExportRouteFenceClassification(t *testing.T) {
	cases := []struct{ method, path, want string }{
		{"GET", "/api/me/data-exports/123/download", "shared"},
		{"POST", "/api/me/data-export-results", "shared"},
		{"PUT", "/api/conversations/id/draft", "exclusive"},
		{"POST", "/api/conversations/id/draft-results", "shared"},
		{"PUT", "/api/conversations/id/read", "exclusive"},
		{"PATCH", "/api/conversations/id/view", "exclusive"},
		{"PATCH", "/api/me/preferences", "exclusive"},
		{"POST", "/api/auth/refresh", "exclusive"},
		{"POST", "/api/auth/login", "exclusive"},
		{"DELETE", "/api/auth/sessions/id", "exclusive"},
		{"PATCH", "/api/me", "exclusive"},
		{"POST", "/api/me/avatar", "exclusive"},
		{"POST", "/api/contacts/requests/id/reject", "exclusive"},
		{"POST", "/api/reports", "exclusive"},
		{"PATCH", "/api/admin/reports/id", "exclusive"},
		{"GET", "/api/reports", ""},
		{"POST", "/api/conversations/id/messages", ""},
		{"GET", "/api/conversations/id/view", ""},
	}
	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if got := personalExportGatewayMode(r); got != tc.want {
			t.Fatalf("%s %s: %s, want %s", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestPersonalExportNestedFenceRequiresAuthenticProof(t *testing.T) {
	a := &App{Role: "message", Config: Config{StateDir: t.TempDir(), Key: bytes.Repeat([]byte{5}, 32)}}
	lock, err := a.admissionFence(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer closeExportFence(lock)
	for _, valid := range []bool{true, false} {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		r := httptest.NewRequest("POST", "/internal/forwards/revoke-principal", nil).WithContext(ctx)
		r.Header.Set("X-JobsIM-Projection-Fence-Mode", "exclusive")
		proof := "forged"
		if valid {
			proof = a.gatewayProjectionProof(r, "exclusive")
		}
		r.Header.Set("X-JobsIM-Projection-Fence", proof)
		called := false
		fn := a.withAdmissionFence("POST /internal/forwards/revoke-principal", func(w http.ResponseWriter, r *http.Request) error {
			called = true
			held, ok := r.Context().Value(heldAdmissionContextKey{}).(heldAdmissionContext)
			if !ok || held.Mode != "exclusive" || held.Identity != a.admissionFenceIdentity() {
				t.Fatal("private call lost original held EX scope")
			}
			return nil
		})
		err := fn(httptest.NewRecorder(), r)
		cancel()
		if valid && (err != nil || !called) {
			t.Fatalf("authentic nested EX self-deadlocked: %v", err)
		}
		if !valid && (err == nil || called) {
			t.Fatal("forged public-like proof skipped lock")
		}
	}
}

func TestPersonalExportSourceBufferDoesNotPartiallyDeliver(t *testing.T) {
	w := &personalExportBufferedResponse{header: make(http.Header)}
	w.Header().Add("Set-Cookie", "private-initial-cookie")
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1<<20)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("overflow")); err == nil || w.err == nil {
		t.Fatal("ignored response overflow could publish partial JSON")
	}
	if w.Header().Get("Set-Cookie") == "" || w.body.Len() != 1<<20 {
		t.Fatal("buffer altered original response before decision")
	}
}

func TestPersonalExportPrivateProofCannotBypassServiceAuthentication(t *testing.T) {
	a := &App{Role: "message", Config: Config{StateDir: t.TempDir(), Key: bytes.Repeat([]byte{9}, 32)}}
	r := httptest.NewRequest("POST", "/internal/forwards/revoke-principal", strings.NewReader(`{}`))
	r.Header.Set("X-JobsIM-Projection-Fence-Mode", "exclusive")
	r.Header.Set("X-JobsIM-Projection-Fence", a.gatewayProjectionProof(r, "exclusive"))
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "SERVICE_AUTH_REQUIRED") {
		t.Fatal("lock proof bypassed private service authentication")
	}
}

func TestPersonalExportProofScopeDrainsInflightAndRejectsLateRPC(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	a := &App{Role: "identity", Config: Config{StateDir: t.TempDir(), Key: bytes.Repeat([]byte{15}, 32)}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !a.hasGatewayProjectionFence(r, true) {
			t.Error("live private RPC has no original exclusive proof")
		}
		close(entered)
		<-release
		respond(w, 200, map[string]bool{"valid": true})
	}))
	defer server.Close()
	a.Client = server.Client()
	a.Config.RoleURLs = map[string]string{"control": server.URL}
	ctx, expire := a.heldAdmissionScope(context.Background(), "exclusive")
	rpcDone := make(chan error, 1)
	go func() { rpcDone <- a.rpc(ctx, "control", "/internal/probe", map[string]bool{}, nil) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("private RPC did not enter")
	}
	expired := make(chan struct{})
	go func() { expire(); close(expired) }()
	select {
	case <-expired:
		t.Fatal("outer scope released while its private RPC still used proof")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-rpcDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-expired:
	case <-time.After(time.Second):
		t.Fatal("scope did not drain")
	}
	err := a.rpc(ctx, "control", "/internal/probe", map[string]bool{}, nil)
	if err == nil || calls.Load() != 1 {
		t.Fatal("released scope emitted another private request")
	}
}

func TestPersonalExportCompleteAuthentication(t *testing.T) {
	key := bytes.Repeat([]byte{17}, 32)
	original := personalExportTestArtifact(t, key)
	goodDigest := sha256.Sum256(original)
	cases := []struct {
		name                  string
		mutate                func([]byte) []byte
		wrongKey, wrongDigest bool
	}{
		{"valid", func(b []byte) []byte { return b }, false, false},
		{"final_tag_corrupt", func(b []byte) []byte { b[len(b)-1] ^= 1; return b }, false, false},
		{"middle_tag_corrupt", func(b []byte) []byte { b[66000] ^= 1; return b }, false, false},
		{"truncated_final", func(b []byte) []byte { return b[:len(b)-9] }, false, false},
		{"trailing_bytes", func(b []byte) []byte { return append(b, 9) }, false, false},
		{"wrong_key", func(b []byte) []byte { return b }, true, false},
		{"wrong_artifact_digest", func(b []byte) []byte { return b }, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.mutate(append([]byte(nil), original...))
			f, err := os.CreateTemp(t.TempDir(), "encrypted-*")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if _, err = f.Write(data); err != nil {
				t.Fatal(err)
			}
			digest := goodDigest[:]
			if tc.wrongDigest {
				digest = bytes.Repeat([]byte{0}, 32)
			}
			actualKey := key
			if tc.wrongKey {
				actualKey = bytes.Repeat([]byte{23}, 32)
			}
			var out bytes.Buffer
			err = authenticatedPersonalZIP(context.Background(), f, actualKey, digest, int64(len(data)), &out, func() error { return nil })
			if tc.name != "valid" {
				if err == nil || out.Len() != 0 {
					t.Fatalf("unauthenticated output escaped: error=%v bytes=%d", err, out.Len())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			reader, err := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
			if err != nil || len(reader.File) != 2 {
				t.Fatalf("not a usable ZIP: %v", err)
			}
			for _, entry := range reader.File {
				stream, err := entry.Open()
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.Copy(io.Discard, stream)
				stream.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestPersonalExportCancelledBeforePlaintext(t *testing.T) {
	key := bytes.Repeat([]byte{21}, 32)
	data := personalExportTestArtifact(t, key)
	digest := sha256.Sum256(data)
	f, err := os.CreateTemp(t.TempDir(), "encrypted-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	calls := 0
	err = authenticatedPersonalZIP(context.Background(), f, key, digest[:], int64(len(data)), &out, func() error {
		calls++
		if calls == 3 {
			return fail(409, "EXPORT_INVALIDATED", "cancelled")
		}
		return nil
	})
	if err == nil || out.Len() != 0 {
		t.Fatal("cancelled first authentication pass disclosed plaintext")
	}
}

func TestPersonalExportOriginalBodyAndNoAuthority(t *testing.T) {
	for _, raw := range []string{`{"requestId":"stable-request","restoreEpoch":"2"}`, `{"requestId":"stable-request","restoreEpoch":"02"}`, `{"requestId":"stable-request","restoreEpoch":2}`, `{"requestId":"stable-request","restoreEpoch":"2","userId":"another"}`, `{"requestId":"stable-request","restoreEpoch":"2"} {}`} {
		q, err := decodePersonalExportCreate(httptest.NewRequest("POST", "/", strings.NewReader(raw)))
		if raw == `{"requestId":"stable-request","restoreEpoch":"2"}` {
			if err != nil || q.RestoreEpoch != "2" {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("accepted ambiguous or expanded original intent")
		}
	}
	value := map[string]any{"ownedText": "My independent text mentions a password without being a credential.", "forwardAdmission": "secret-proof", "nested": []any{map[string]any{"replyAdmission": "proof", "token": "secret", "id": "ordinary-id"}}, "recovery": map[string]any{"ownedText": "unpublished"}}
	stripPersonalExportAuthority(value)
	raw, _ := json.Marshal(value)
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "proof") || strings.Contains(string(raw), "unpublished") || !strings.Contains(string(raw), "independent text") || !strings.Contains(string(raw), "ordinary-id") {
		t.Fatal("authority scrub removed own text or retained temporary authority")
	}
}
