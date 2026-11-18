package jobsim

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"testing"
)

func actualWebMBrowserFixture(t *testing.T) (string, []byte) {
	t.Helper()
	file, want := os.Getenv("JOBS_IM_WEBM_BROWSER_FIXTURE"), os.Getenv("JOBS_IM_WEBM_BROWSER_SHA256")
	if file == "" || want == "" {
		t.Skip("explicit original browser Blob and its fixed SHA256 required")
	}
	info, err := os.Stat(file)
	if err != nil || !info.Mode().IsRegular() || info.Size() > mediaUploadFileLimit {
		t.Fatal("bounded original browser fixture unavailable")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal("original browser fixture unreadable")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != want {
		t.Fatal("original browser fixture digest changed")
	}
	return file, data
}

func TestWebMActualBrowserContainerClassification(t *testing.T) {
	file, data := actualWebMBrowserFixture(t)
	if len(data) > 512 {
		data = data[:512]
	}
	got, err := classifyStoredMedia(file, http.DetectContentType(data))
	if err != nil || got != "audio/webm" {
		t.Fatalf("actual browser audio-only container got %q, %v", got, err)
	}
}

func TestMediaUploadPGActualBrowserContainerRemainsAudio(t *testing.T) {
	_, data := actualWebMBrowserFixture(t)
	f := newDeletionPG(t, 7)
	a := f.nodes["media"]
	q, _ := mediaUploadPGRequest(t, f, data)
	q.Name = "original-browser-voice.webm"
	result := deletionBody(t, mediaUploadPGCall(t, a, f.token, &q, q.Name, data), 200)
	if result["contentType"] != "audio/webm" || result["sha256"] != q.Digest {
		t.Fatal("actual browser original was not committed as exact audio/WebM")
	}
	query := deletionBody(t, deletionCall(a, "/api/media/upload-results", f.token, q), 200)
	if query["id"] != result["id"] || query["contentType"] != "audio/webm" {
		t.Fatal("actual browser original receipt changed after upload")
	}
}
