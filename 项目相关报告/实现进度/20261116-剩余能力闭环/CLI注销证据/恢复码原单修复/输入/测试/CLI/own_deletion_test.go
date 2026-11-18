// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type cliDeletionFixture struct {
	*cliPasswordFixture
	value                         ownDeletionResult
	secret                        ownDeletionSecret
	available                     bool
	epoch                         string
	creates, cancels, queries     int
	lost, prepared403, cancelLost bool
	proofTamper                   func(*ownDeletionProof)
	beforeCreate                  func()
}

func cliDeletionSetup(t *testing.T) *cliDeletionFixture {
	p := cliPasswordSetup(t)
	yes := true
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	r := &cliDeletionFixture{cliPasswordFixture: p, available: true, epoch: "1", secret: ownDeletionSecret{Password: []byte("Original-password-123")}, value: ownDeletionResult{ID: "d1111111-1111-4111-8111-111111111111", State: "requested", Version: "1", Epoch: "1", Until: &until, Login: &yes}}
	if e := ownDeletionMigrate(r.db); e != nil {
		t.Fatal(e)
	}
	p.setHook(r.serveDeletion)
	return r
}
func (r *cliDeletionFixture) serveDeletion(w http.ResponseWriter, q *http.Request) bool {
	switch q.URL.Path {
	case "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": r.identity.DeploymentID, "restoreEpoch": r.epoch, "features": map[string]bool{"own_account_deletion": r.available}})
	case "/api/auth/deletion-results", "/api/auth/deletion-requests":
		var body map[string]string
		raw, _ := io.ReadAll(q.Body)
		if json.Unmarshal(raw, &body) != nil {
			r.t.Error("bad request")
		}
		if q.URL.Path == "/api/auth/deletion-results" {
			if q.Header.Get("Authorization") != "" || q.Header.Get("Cookie") != "" {
				r.t.Error("original result must be anonymous")
			}
			if r.recorded {
				r.value.Request = body["requestId"]
				cliSyncJSON(w, 200, r.value)
			} else {
				cliSyncJSON(w, 404, map[string]any{"error": map[string]string{"code": "ACCOUNT_DELETION_NOT_FOUND"}})
			}
			return true
		}
		r.creates++
		if r.beforeCreate != nil {
			r.beforeCreate()
		}
		r.recorded = true
		r.value.Request = body["requestId"]
		if r.prepared403 {
			cliSyncJSON(w, 403, map[string]any{"error": map[string]string{"code": "DEPENDENCY_FORBIDDEN"}})
		} else if r.lost {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "DELIVERY_UNKNOWN"}})
		} else {
			cliSyncJSON(w, 200, r.value)
		}
	case "/api/auth/data-rights/proofs":
		if q.Header.Get("Authorization") != "" || q.Header.Get("Cookie") != "" {
			r.t.Error("restricted proof borrowed ordinary credentials")
		}
		var body map[string]string
		_ = json.NewDecoder(q.Body).Decode(&body)
		proof := ownDeletionProof{Token: strings.Repeat("p", 48), ID: r.value.ID, Version: r.value.Version, Purpose: body["purpose"], Epoch: r.epoch, Expires: time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)}
		if r.proofTamper != nil {
			r.proofTamper(&proof)
		}
		cliSyncJSON(w, 200, proof)
	case "/api/auth/data-rights/query":
		r.queries++
		cliSyncJSON(w, 200, r.value)
	case "/api/auth/data-rights/cancel":
		r.cancels++
		var body map[string]string
		_ = json.NewDecoder(q.Body).Decode(&body)
		r.value.Cancel = body["requestId"]
		r.value.Version = "2"
		r.value.State = "cancelled"
		if r.cancelLost {
			cliSyncJSON(w, 503, map[string]any{"error": map[string]string{"code": "CANCEL_UNKNOWN"}})
		} else {
			cliSyncJSON(w, 200, r.value)
		}
	default:
		return false
	}
	return true
}
func (r *cliDeletionFixture) prepareDeletion() ownDeletionIntent {
	o, e := r.client.prepareOwnDeletion()
	if e != nil {
		r.t.Fatal(e)
	}
	return o
}
func (r *cliDeletionFixture) loadDeletion(id string) ownDeletionIntent {
	o, e := loadOwnDeletion(r.db, id, r.client.base, r.client.deployment, r.identity.ConnectionProfile, "password_user")
	if e != nil {
		r.t.Fatal(e)
	}
	return o
}
func (r *cliDeletionFixture) requestDeletion() ownDeletionIntent {
	o := r.prepareDeletion()
	if _, e := r.client.performOwnDeletion(r.db, o, &r.secret, false, true); e != nil {
		r.t.Fatal(e)
	}
	return r.loadDeletion(o.Meta.ID)
}
func TestOwnDeletionDurableBeforeHTTPAndColdGateWithoutSecrets(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.prepareDeletion()
	r.beforeCreate = func() {
		saved := r.loadDeletion(o.Meta.ID)
		if saved.Attempts != 1 || saved.State != "unknown" || r.client.auth.state.Status != "account_deletion_unknown" {
			t.Error("public responsibility not committed before mutation")
		}
	}
	r.lost = true
	if _, e := r.client.performOwnDeletion(r.db, o, &r.secret, false, true); e == nil {
		t.Fatal("unknown accepted")
	}
	raw, _ := os.ReadFile(filepath.Join(r.root, "JobsIM", "CLI", "Authentication", r.identity.ConnectionProfile, fingerprint("password_user"), "authentication.sqlite"))
	if strings.Contains(string(raw), string(r.secret.Password)) {
		t.Fatal("password leaked to SQLite")
	}
	r.client.auth.state.Status = "active"
	if e := r.client.auth.ownDeletionRestoreGate(); e != nil || r.client.auth.state.Status == "active" {
		t.Fatal("cold ordinary credentials reopened", e)
	}
	if _, e := r.client.auth.snapshot(); e == nil {
		t.Fatal("unknown family snapshot allowed")
	}
}
func TestOwnDeletionLostCommitReconcilesAnonymouslyWithoutSecondCreate(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.prepareDeletion()
	r.lost = true
	_, _ = r.client.performOwnDeletion(r.db, o, &r.secret, false, true)
	r.lost = false
	if _, e := r.client.performOwnDeletion(r.db, r.loadDeletion(o.Meta.ID), &r.secret, false, false); e != nil {
		t.Fatal(e)
	}
	if r.creates != 1 || r.client.token != "" || r.client.auth.state.Status != "account_deletion_stopped" {
		t.Fatal("original result created or revived credentials")
	}
}
func TestOwnDeletionCapabilityWithdrawalAndSuccessorCannotCreate(t *testing.T) {
	for _, changedEpoch := range []bool{false, true} {
		r := cliDeletionSetup(t)
		o := r.prepareDeletion()
		r.recorded = true
		r.value.State = "pending"
		r.value.Request = o.Meta.ID
		r.available = false
		if changedEpoch {
			r.epoch = "2"
		}
		if _, e := r.client.performOwnDeletion(r.db, o, &r.secret, false, false); e != nil {
			t.Fatal(e)
		}
		if _, e := r.client.performOwnDeletion(r.db, r.loadDeletion(o.Meta.ID), &r.secret, true, false); e == nil || r.creates != 0 {
			t.Fatal("withdrawn capability mutated")
		}
	}
	r := cliDeletionSetup(t)
	o := r.prepareDeletion()
	r.client.auth.pair.FamilyID = "e1111111-1111-4111-8111-111111111111"
	if _, e := r.client.performOwnDeletion(r.db, o, &r.secret, true, false); e == nil || r.creates != 0 {
		t.Fatal("unseen borrowed successor family")
	}
}
func TestOwnDeletionPreparedDependency403RetainsUnknownGate(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.prepareDeletion()
	r.prepared403 = true
	if _, e := r.client.performOwnDeletion(r.db, o, &r.secret, false, true); e == nil {
		t.Fatal("dependency failure accepted")
	}
	if r.loadDeletion(o.Meta.ID).State != "unknown" || r.client.auth.state.Status != "account_deletion_unknown" {
		t.Fatal("generic403 removed original gate")
	}
}
func TestOwnDeletionProofPurposeEpochVersionAndExpiryRejectBeforeAction(t *testing.T) {
	variants := []func(*ownDeletionProof){func(p *ownDeletionProof) { p.Purpose = "cancel" }, func(p *ownDeletionProof) { p.Epoch = "2" }, func(p *ownDeletionProof) { p.Version = "0" }, func(p *ownDeletionProof) { p.Expires = time.Now().Add(-time.Minute).Format(time.RFC3339Nano) }}
	for _, tamper := range variants {
		r := cliDeletionSetup(t)
		o := r.requestDeletion()
		r.proofTamper = tamper
		if _, e := r.client.ownDeletionRestricted(r.db, o, &r.secret, false, false); e == nil || r.queries != 0 || r.cancels != 0 {
			t.Fatal("bad proof reached action")
		}
	}
}
func TestOwnDeletionUnknownCancelQueriesOriginalAndNeverRebasesNonce(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.requestDeletion()
	r.cancelLost = true
	if _, e := r.client.ownDeletionRestricted(r.db, o, &r.secret, true, false); e == nil {
		t.Fatal("lost cancel accepted")
	}
	saved := r.loadDeletion(o.Meta.ID)
	if saved.Cancel == "" || saved.CancelState != "unknown" {
		t.Fatal("missing durable cancellation nonce")
	}
	if _, e := r.client.ownDeletionRestricted(r.db, saved, &r.secret, true, false); e == nil || r.cancels != 1 {
		t.Fatal("unknown silently repeated")
	}
	if _, e := r.client.ownDeletionRestricted(r.db, saved, &r.secret, true, true); e != nil {
		t.Fatal(e)
	}
	after := r.loadDeletion(o.Meta.ID)
	if after.Cancel != saved.Cancel || after.State != "cancelled" || r.cancels != 1 || r.queries != 1 {
		t.Fatal("unknown cancellation did not recover by pure original query")
	}
	raw, _ := json.Marshal(after)
	if strings.Contains(string(raw), strings.Repeat("p", 48)) {
		t.Fatal("restricted proof persisted")
	}
}
func TestOwnDeletionCancelObservedVersionNeverSilentlyAdvances(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.requestDeletion()
	next := o
	next.Cancel = "f1111111-1111-4111-8111-111111111111"
	next.CancelVersion = "1"
	next.CancelState = "unknown"
	o, e := saveOwnDeletion(r.db, o, next)
	if e != nil {
		t.Fatal(e)
	}
	r.value.Version = "3"
	if _, e = r.client.ownDeletionRestricted(r.db, o, &r.secret, true, true); e == nil || r.cancels != 0 {
		t.Fatal("cancel rebased to newer query version")
	}
	if r.loadDeletion(o.Meta.ID).Cancel != next.Cancel {
		t.Fatal("original nonce replaced")
	}
}
func TestOwnDeletionCompletedCleansAllOwnedStoresAndManagedBytesOnly(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.requestDeletion()
	base := filepath.Join(r.root, "JobsIM", "CLI", r.identity.ConnectionProfile)
	makeStore := func(identity localIdentity, epoch string) *localStore {
		raw, _ := json.Marshal(identity)
		s, e := openStore(filepath.Join(base, fingerprint(string(raw)), "client.sqlite"), identity, epoch)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.db.Exec("INSERT INTO drafts VALUES(?,?,?,?)", epoch, r.conversation, "private account draft", "now"); e != nil {
			t.Fatal(e)
		}
		return s
	}
	own := makeStore(r.identity, "1")
	r.client.store = own
	otherEpoch := r.identity
	otherEpoch.DeviceID = "a2222222-2222-4222-8222-222222222222"
	old := makeStore(otherEpoch, "2")
	defer old.close()
	other := r.identity
	other.UserID = "b2222222-2222-4222-8222-222222222222"
	neighbor := makeStore(other, "1")
	defer neighbor.close()
	private := filepath.Join(filepath.Dir(own.path), "ReceiptObjects", "1", "c2222222-2222-4222-8222-222222222222.bin")
	_ = os.MkdirAll(filepath.Dir(private), 0700)
	_ = os.WriteFile(private, []byte("managed original bytes"), 0600)
	external := filepath.Join(r.root, "explicit-user-export.zip")
	_ = os.WriteFile(external, []byte("user external bytes"), 0600)
	r.value.State = "completed"
	r.value.Version = "2"
	if _, e := r.client.applyOwnDeletion(r.db, o, r.value); e != nil {
		t.Fatal(e)
	}
	if !r.loadDeletion(o.Meta.ID).Cleared {
		t.Fatal("cleanup not acknowledged")
	}
	if _, e := os.Stat(private); !os.IsNotExist(e) {
		t.Fatal("managed original survived")
	}
	if _, e := os.Stat(external); e != nil {
		t.Fatal("external file removed")
	}
	var count int
	if e := neighbor.db.QueryRow("SELECT count(*) FROM drafts").Scan(&count); e != nil || count != 1 {
		t.Fatal("other user erased", e)
	}
	if _, e := old.db.Exec("INSERT INTO drafts VALUES('2','late','resurrection','now')"); e == nil {
		t.Fatal("late writer resurrected cleared table")
	}
	if _, e := openStore(own.path, r.identity, "1"); e == nil {
		t.Fatal("terminal account reopened")
	}
}
func TestOwnDeletionLateOriginalResultDoesNotClearSuccessor(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.prepareDeletion()
	r.value.Request = o.Meta.ID
	r.client.auth.pair.FamilyID = "d2222222-2222-4222-8222-222222222222"
	r.client.auth.state.FamilyID = r.client.auth.pair.FamilyID
	before := r.client.token
	if _, e := r.client.applyOwnDeletion(r.db, o, r.value); e != nil {
		t.Fatal(e)
	}
	if r.client.token != before || r.client.auth.pair.FamilyID == "" {
		t.Fatal("late old result cleared successor")
	}
}
func TestOwnDeletionMetadataCASAndTypedResultBindOriginal(t *testing.T) {
	r := cliDeletionSetup(t)
	if fmt.Sprint(r.secret) != "ownDeletionSecret(redacted)" || fmt.Sprint(ownDeletionProof{Token: "never-log-proof"}) != "ownDeletionProof(redacted)" {
		t.Fatal("secret diagnostic exposed fields")
	}
	o := r.prepareDeletion()
	next := o
	next.Attempts = 1
	next.State = "unknown"
	saved, e := saveOwnDeletion(r.db, o, next)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = saveOwnDeletion(r.db, o, next); e == nil {
		t.Fatal("stale SQLite CAS accepted")
	}
	v := r.value
	v.Request = o.Meta.ID
	v.Epoch = "2"
	if _, e = r.client.applyOwnDeletion(r.db, saved, v); e == nil {
		t.Fatal("cross epoch result installed")
	}
	raw, _ := json.Marshal(saved)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	fields["password"] = "must-reject"
	raw, _ = json.Marshal(fields)
	if _, e = r.db.Exec("UPDATE own_deletion_intents SET payload=? WHERE id=?", string(raw), o.Meta.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = loadOwnDeletion(r.db, o.Meta.ID, r.client.base, r.client.deployment, r.identity.ConnectionProfile, "password_user"); e == nil {
		t.Fatal("secret-bearing metadata accepted")
	}
}

func TestOwnDeletionUnknownCancelRecoveryCodeIsNotConsumedAgainForCancel(t *testing.T) {
	r := cliDeletionSetup(t)
	o := r.requestDeletion()
	next := o
	next.Cancel = "e3333333-3333-4333-8333-333333333333"
	next.CancelVersion = "1"
	next.CancelState = "unknown"
	o, e := saveOwnDeletion(r.db, o, next)
	if e != nil {
		t.Fatal(e)
	}
	proof := ownDeletionSecret{Recovery: []byte("one-use-recovery-fixture")}
	defer proof.clear()
	if _, e = r.client.ownDeletionRestricted(r.db, o, &proof, true, true); e == nil || !strings.Contains(e.Error(), "REVERIFY_REQUIRED") || r.queries != 1 || r.cancels != 0 {
		t.Fatal("one-use query code was reused for cancel", e)
	}
	checked := r.loadDeletion(o.Meta.ID)
	if checked.CancelState != "prepared" || checked.Cancel != next.Cancel {
		t.Fatal("original checked cancel responsibility lost")
	}
	proof.Recovery = []byte("different-recovery-fixture")
	if _, e = r.client.ownDeletionRestricted(r.db, checked, &proof, true, true); e != nil || r.cancels != 1 || r.queries != 1 {
		t.Fatal("new proof could not continue original checked cancel", e)
	}
}
