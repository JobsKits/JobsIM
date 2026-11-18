package jobsim

import (
	"encoding/json"
	"testing"
)

// Real PG and production handlers; RPC is confined in-process and the NATS
// configuration flag is a fixture. The live HTTPS demo is verified separately.
func TestSyncSnapshotPGDirectSelfGroupCaptureAndRedactedGate(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, peer, token)
	direct := uiRegressionDirect(t, f, peer)
	self := deletionBody(t, uiRegressionCall(f.nodes["conversation"], "POST", "/api/conversations", f.token, map[string]any{"kind": "self"}), 200)
	group := deletionBody(t, uiRegressionCall(f.nodes["conversation"], "POST", "/api/conversations", f.token, map[string]any{"kind": "group", "title": "Announcement capture fixture", "memberIds": []string{peer.ID}}), 200)
	sync := f.nodes["sync"]
	sync.Config.NATSURL = "nats://fixture.invalid:1"
	store := newID()
	deletionBody(t, uiRegressionCall(sync, "PUT", "/api/sync/device", f.token, map[string]any{"requestId": newID(), "storeGeneration": store, "schemaVersion": "2"}), 200)
	created := deletionBody(t, uiRegressionCall(sync, "POST", "/api/sync/snapshots", f.token, map[string]any{"requestId": newID(), "storeGeneration": store, "schemaVersion": "2", "pageSize": 100}), 201)
	id := created["snapshotId"].(string)
	page := deletionBody(t, uiRegressionCall(sync, "GET", "/api/sync/snapshots/"+id+"/pages/0", f.token, nil), 200)
	if page["installable"] != true {
		t.Fatal("direct/self/group baseline is not installable")
	}
	encoded, _ := json.Marshal(page["refs"])
	var refs []syncRef
	if json.Unmarshal(encoded, &refs) != nil {
		t.Fatal("reference fixture unavailable")
	}
	seen := map[string]bool{}
	announcements := []string{}
	for _, ref := range refs {
		if ref.Kind == "conversation" {
			seen[ref.ID] = true
		}
		if ref.Kind == "announcement" {
			announcements = append(announcements, ref.ID)
		}
	}
	if !seen[direct["id"].(string)] || !seen[self["id"].(string)] || !seen[group["id"].(string)] {
		t.Fatal("normal conversation capture was removed")
	}
	if len(announcements) != 1 || announcements[0] != group["id"].(string) {
		t.Fatal("only group announcement references belong in this baseline")
	}
	// A previously captured forbidden reference remains non-installable. This
	// tests the real page gate instead of relaxing client/server validation.
	refs = append(refs, syncRef{Owner: "conversation", Kind: "announcement", ID: direct["id"].(string), ConversationID: direct["id"].(string), Revision: "0", RevisionEpoch: "1", State: "active"})
	raw, _ := json.Marshal(refs)
	cipher, err := sync.seal(string(raw), "sync-refs:"+id+":0")
	if err != nil {
		t.Fatal(err)
	}
	digest := syncDigest(raw)
	manifestBytes, _ := json.Marshal(created)
	var manifest syncSnapshot
	if json.Unmarshal(manifestBytes, &manifest) != nil {
		t.Fatal("original manifest fixture unavailable")
	}
	manifest.PageManifest[0].Digest = digest
	manifest.PageManifest[0].Count = len(refs)
	manifestBytes, _ = json.Marshal(manifest)
	deletionExec(t, f.ctx, sync, `UPDATE sync.snapshots SET manifest=$2 WHERE id=$1`, id, manifestBytes)
	deletionExec(t, f.ctx, sync, `UPDATE sync.snapshot_pages SET refs_ciphertext=$2,refs_digest=$3 WHERE snapshot_id=$1 AND page=0`, id, cipher, digest)
	page = deletionBody(t, uiRegressionCall(sync, "GET", "/api/sync/snapshots/"+id+"/pages/0", f.token, nil), 200)
	if page["installable"] != false {
		t.Fatal("redacted historical reference bypassed installability gate")
	}
}
