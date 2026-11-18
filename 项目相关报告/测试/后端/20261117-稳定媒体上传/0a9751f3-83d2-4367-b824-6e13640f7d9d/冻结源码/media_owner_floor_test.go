package jobsim

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaRetentionPGProcessingOwnerFloorDeniesUnqueuedOriginals(t *testing.T) {
	ctx, a, f, data := newMediaRetentionPG(t)
	avatar, unbound, neighbor := newID(), newID(), newID()
	for _, row := range []struct{ id, owner, purpose string }{{avatar, f.user.ID, "profile_avatar"}, {unbound, f.user.ID, "chat_attachment"}, {neighbor, newID(), "chat_attachment"}} {
		retentionExec(t, ctx, a, `INSERT INTO media.objects(id,owner_id,name,content_type,size_bytes,sha256,storage_key,purpose) VALUES($1::uuid,$2,'owned original','text/plain',$3,$4,$1::uuid::text,$5)`, row.id, row.owner, len(data), make([]byte, 32), row.purpose)
		if err := os.WriteFile(filepath.Join(a.Config.StateDir, "Media", row.id), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Leave every file untouched and no object-specific retirement task queued.
	retentionExec(t, ctx, a, `INSERT INTO media.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,2)`, f.user.ID, newID())
	for _, path := range []string{"/api/media/" + f.plan.Object, "/api/media/" + unbound, "/api/avatars/" + avatar} {
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 410 || bytes.Contains(w.Body.Bytes(), data) {
			t.Fatalf("processing owner floor did not deny %s: %d", path, w.Code)
		}
	}
	if err := a.requireMediaRetention(ctx, f.plan.Message, f.plan.Object); err == nil {
		t.Fatal("forward/export source helper bypassed owner floor")
	}
	body, _ := json.Marshal(map[string]string{"userId": f.user.ID, "deviceId": f.user.DeviceID, "storeGeneration": f.store, "restoreEpoch": "1", "contentVersion": "1", "messageId": f.plan.Message, "objectId": f.plan.Object})
	r := httptest.NewRequest("POST", "/internal/receipts/object", bytes.NewReader(body))
	r.Header.Set("X-JobsIM-Service-Key", encodeTestServiceKey(a.Config.Key))
	r.Header.Set("X-JobsIM-Caller", "sync")
	r.Header.Set("X-JobsIM-Projection-Fence-Mode", "shared")
	r.Header.Set("X-JobsIM-Projection-Fence", a.gatewayProjectionProof(r, "shared"))
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != 410 {
		t.Fatalf("receipt descriptor escaped owner floor: %d %s", w.Code, w.Body.String())
	}
	if err := a.requireMediaOwnerReadable(ctx, neighbor); err != nil {
		t.Fatal("another owner's original was denied", err)
	}
	for _, id := range []string{f.plan.Object, avatar, unbound, neighbor} {
		got, err := os.ReadFile(filepath.Join(a.Config.StateDir, "Media", id))
		if err != nil || !bytes.Equal(got, data) {
			t.Fatal("read denial changed an unprocessed file", err)
		}
	}
}
