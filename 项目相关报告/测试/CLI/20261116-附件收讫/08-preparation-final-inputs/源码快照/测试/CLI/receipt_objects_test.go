// Created by Jobs on 2026年11月16日，星期一.
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type cliObjectReceiptFixture struct {
	*cliReceiptFixture
	parent          string
	data            []byte
	object          string
	challenge       receiptObjectChallenge
	result          json.RawMessage
	resultStatus    int
	submitStatus    int
	objectFeature   bool
	beforeSubmit    func(receiptOperation)
	beforeChallenge func()
	beforeMedia     func()
}

func cliObjectSetup(t *testing.T) *cliObjectReceiptFixture {
	r := cliReceiptSetup(t)
	r.storage.Contract = receiptFileContract
	r.storage.ObjectQualified = true
	o := &cliObjectReceiptFixture{cliReceiptFixture: r, data: []byte("original\x00file\nbytes\xff"), object: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", resultStatus: 404, submitStatus: 200, objectFeature: true}
	var m map[string]any
	if syncDecode(r.message(), &m) != nil {
		t.Fatal("message fixture")
	}
	m["type"], m["attachmentId"] = "file", o.object
	raw, _ := json.Marshal(m)
	r.challenge = r.makeChallenge(raw, time.Now().Add(30*time.Minute).Truncate(time.Second))
	r.setHook(o.serve)
	if _, err := r.client.receiveReceiptMessage(r.messageID, false); err != nil {
		t.Fatal(err)
	}
	o.parent = r.receiptID()
	// Install the same media DTO through the ordinary authoritative projection,
	// rather than retaining the setup helper's earlier text eligibility digest.
	ref := r.ref("message", r.messageID)
	projectionRaw, _ := json.Marshal(r.projection(ref, raw, "available"))
	projection, e := syncReadProjection(projectionRaw, ref, "1")
	if e != nil {
		t.Fatal(e)
	}
	control := r.control()
	tx, e := r.store.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if e = r.store.applySync(tx, projection, control); e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	parent, err := r.store.loadReceipt(o.parent)
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	o.challenge = receiptObjectChallenge{Parent: o.parent, Message: parent.Message, Conversation: parent.Conversation, Content: parent.Content, Epoch: "1", Store: r.physical, Qualification: r.qualification, Digest: parent.Digest, Object: receiptObject{ID: o.object, Size: strconv.Itoa(len(o.data)), Digest: fingerprint(string(o.data)), ContentType: "application/octet-stream"}, Expires: time.Now().Add(30 * time.Minute).Truncate(time.Second).Format(time.RFC3339Nano), Durable: &yes, Complete: &no, Target: &no, Deletion: &no}
	o.sign()
	return o
}
func (o *cliObjectReceiptFixture) sign() {
	expires, _ := time.Parse(time.RFC3339Nano, o.challenge.Expires)
	v := o.challenge
	p := receiptObjectProof{receiptProofTuple: receiptProofTuple{Scope: o.identity.DeploymentID, User: o.identity.UserID, Device: o.identity.DeviceID, Store: v.Store, Epoch: v.Epoch, Qualification: v.Qualification, Message: v.Message, Conversation: v.Conversation, Content: v.Content, Digest: v.Digest, Permission: o.permission, Expires: expires.Unix()}, Parent: v.Parent, Object: v.Object.ID, Size: v.Object.Size, ObjectDigest: v.Object.Digest}
	raw, _ := json.Marshal(p)
	o.challenge.Challenge = "ro1." + base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
}
func (o *cliObjectReceiptFixture) serve(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/bootstrap":
		cliSyncJSON(w, 200, map[string]any{"deploymentId": o.identity.DeploymentID, "restoreEpoch": "1", "features": map[string]bool{"authoritative_sync": true, "sync_jetstream": true, "durable_receipt_protocol": o.feature, "durable_object_receipt_protocol": o.objectFeature}})
	case "/api/sync/object-receipt-challenges":
		if o.beforeChallenge != nil {
			o.beforeChallenge()
		}
		cliSyncJSON(w, 200, o.challenge)
	case "/api/media/" + o.object:
		if o.beforeMedia != nil {
			o.beforeMedia()
		}
		w.Write(o.data)
	case "/api/sync/object-receipt-results":
		if o.resultStatus == 200 {
			w.Header().Set("Content-Type", "application/json")
			w.Write(o.result)
		} else {
			cliSyncJSON(w, o.resultStatus, map[string]any{"error": map[string]string{"code": "OBJECT_RECEIPT_OPERATION_NOT_FOUND"}})
		}
	case "/api/sync/object-receipts":
		operation, err := o.store.loadReceipt(o.receiptID())
		if err != nil {
			o.t.Error(err)
		}
		if o.beforeSubmit != nil {
			o.beforeSubmit(operation)
		}
		if o.submitStatus != 200 {
			cliSyncJSON(w, o.submitStatus, map[string]any{"error": map[string]string{"code": "TEST_UNKNOWN"}})
			return true
		}
		d := operation.Object
		v := map[string]any{"operationId": operation.ID, "status": "recorded", "receiptKind": "object_received", "parentReceiptId": d.Parent, "messageId": operation.Message, "conversationId": operation.Conversation, "contentVersion": operation.Content, "restoreEpoch": "1", "deviceId": o.identity.DeviceID, "storeGeneration": o.physical, "qualificationId": o.qualification, "objectId": d.Object, "sizeBytes": d.Size, "objectDigest": d.Digest, "bodyDigest": operation.Digest, "storageQualifiedAtCommit": true, "objectComplete": true, "targetRangeComplete": false, "deletionEligible": false}
		o.result, _ = json.Marshal(v)
		o.resultStatus = 200
		w.Header().Set("Content-Type", "application/json")
		w.Write(o.result)
	default:
		return o.serveReceipt(w, r)
	}
	return true
}
func (o *cliObjectReceiptFixture) intent() receiptOperation {
	v, err := o.store.loadReceipt(o.receiptID())
	if err != nil {
		o.t.Fatal(err)
	}
	if v.Kind != "object_received" {
		o.t.Fatalf("kind %s", v.Kind)
	}
	return v
}
func (o *cliObjectReceiptFixture) reopen() {
	path := o.store.path
	o.store.close()
	s, err := openStore(path, o.identity, "1")
	if err != nil {
		o.t.Fatal(err)
	}
	o.store = s
	o.client.store = s
	o.t.Cleanup(func() { s.close() })
}
func TestObjectReceiptFullFileAndDescriptorCommittedBeforeHTTP(t *testing.T) {
	o := cliObjectSetup(t)
	if err := o.store.enqueue(o.pending()); err != nil {
		t.Fatal(err)
	}
	o.beforeSubmit = func(v receiptOperation) {
		if err := o.store.checkReceiptObjectDescriptor(v); err != nil {
			t.Error(err)
		}
		file, err := o.store.openReceiptObject(*v.Object)
		if err != nil {
			t.Error(err)
		} else {
			file.Close()
		}
		if strings.Contains(v.Body, "original") || strings.Contains(v.Body, "text") || strings.Contains(v.Body, "bodyEnvelope") {
			t.Error("file/body bytes copied to receipt intent")
		}
		if err := receiptDurability(o.store.db); err != nil {
			t.Error(err)
		}
	}
	raw, err := o.client.receiveReceiptObject(o.parent, "")
	if err != nil {
		t.Fatal(err)
	}
	v := o.intent()
	if v.State != "completed" || receiptValidateObjectResult(raw, v) != nil {
		t.Fatal("not completed")
	}
	var state string
	if err = o.store.db.QueryRow("SELECT state FROM outbox WHERE client_msg_id=?", o.pending().ClientMsgID).Scan(&state); err != nil || state != "pending" {
		t.Fatal("receipt must not ACK message", err, state)
	}
	if filepath.IsAbs(v.Object.Path) || strings.Contains(v.Object.Path, "..") {
		t.Fatal("not portable relative path")
	}
}
func TestObjectReceiptUnknownReopenPureQueryAndExplicitSameBody(t *testing.T) {
	o := cliObjectSetup(t)
	o.submitStatus = 503
	if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
		t.Fatal("unknown accepted")
	}
	v := o.intent()
	if v.State != "pending" {
		t.Fatal(v.State)
	}
	before := len(o.calls("/api/sync/object-receipts"))
	o.reopen()
	if _, err := o.client.performReceipt(v.ID, false, false); err == nil {
		t.Fatal("404 accepted")
	}
	if len(o.calls("/api/sync/object-receipts")) != before {
		t.Fatal("pure query submitted")
	}
	o.submitStatus = 200
	if _, err := o.client.performReceipt(v.ID, true, false); err != nil {
		t.Fatal(err)
	}
	latest := o.intent()
	if latest.Body != v.Body || latest.Hash != v.Hash || latest.Object.Path != v.Object.Path {
		t.Fatal("original changed")
	}
	if _, err := o.client.performReceipt(v.ID, true, false); err != nil {
		t.Fatal(err)
	}
	if len(o.calls("/api/sync/object-receipts")) != before+1 {
		t.Fatal("completed was replayed")
	}
}
func TestObjectReceiptWrongHashCountAndImportNeverMovesOriginal(t *testing.T) {
	for _, kind := range []string{"hash", "short", "long", "good"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			data := bytes.Clone(o.data)
			switch kind {
			case "hash":
				data[0] ^= 1
			case "short":
				data = data[:len(data)-1]
			case "long":
				data = append(data, 0)
			}
			source := filepath.Join(t.TempDir(), "user file.bin")
			if err := os.WriteFile(source, data, 0644); err != nil {
				t.Fatal(err)
			}
			_, err := o.client.receiveReceiptObject(o.parent, source)
			if (err == nil) != (kind == "good") {
				t.Fatalf("%s: %v", kind, err)
			}
			after, e := os.ReadFile(source)
			if e != nil || !bytes.Equal(data, after) {
				t.Fatal("source modified")
			}
			if kind != "good" && len(o.calls("/api/sync/object-receipts")) != 0 {
				t.Fatal("bad file acknowledged")
			}
		})
	}
}
func TestObjectReceiptSQLFailureAndOrphanCrashBoundary(t *testing.T) {
	t.Run("process-exit-after-FULL-commit", func(t *testing.T) {
		type seed struct {
			Path      string
			Identity  localIdentity
			Operation receiptOperation
		}
		if output := os.Getenv("JOBSIM_OBJECT_RECEIPT_CRASH_FIXTURE"); output != "" {
			o := cliObjectSetup(t)
			o.submitStatus = 503
			if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
				t.Fatal("expected unknown")
			}
			v := seed{Path: o.store.path, Identity: o.identity, Operation: o.intent()}
			raw, _ := json.Marshal(v)
			if err := os.WriteFile(output, raw, 0600); err != nil {
				t.Fatal(err)
			}
			// Deliberately bypass close/cleanup after actual durable SQL and file writes.
			os.Exit(91)
		}
		path := filepath.Join(t.TempDir(), "crash.json")
		cmd := exec.Command(os.Args[0], "-test.run=^TestObjectReceiptSQLFailureAndOrphanCrashBoundary$/process-exit-after-FULL-commit$", "-test.timeout=20s")
		cmd.Env = append(os.Environ(), "JOBSIM_OBJECT_RECEIPT_CRASH_FIXTURE="+path, "TMPDIR="+filepath.Dir(path))
		raw, err := cmd.CombinedOutput()
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 91 {
			t.Fatalf("crash child: %v %s", err, raw)
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var v seed
		if json.Unmarshal(encoded, &v) != nil {
			t.Fatal("fixture metadata")
		}
		// The helper's TempDir is isolated and survived the deliberate os.Exit.
		s, err := openStore(v.Path, v.Identity, "1")
		if err != nil {
			t.Fatal(err)
		}
		defer s.close()
		got, err := s.loadReceipt(v.Operation.ID)
		if err != nil || got.Body != v.Operation.Body || got.Hash != v.Operation.Hash || got.State != "pending" {
			t.Fatal("durable original lost", err)
		}
		if err = s.checkReceiptObjectDescriptor(got); err != nil {
			t.Fatal(err)
		}
		file, err := s.openReceiptObject(*got.Object)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	})
	o := cliObjectSetup(t)
	if _, err := o.store.db.Exec("CREATE TRIGGER fail_object_receipt BEFORE INSERT ON receipt_outbox WHEN json_extract(NEW.payload,'$.kind')='object_received' BEGIN SELECT RAISE(FAIL,'test commit failure'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
		t.Fatal("SQL fault accepted")
	}
	var n int
	if err := o.store.db.QueryRow("SELECT count(*) FROM receipt_objects").Scan(&n); err != nil || n != 0 {
		t.Fatal("descriptor escaped transaction", err, n)
	}
	if len(o.calls("/api/sync/object-receipts")) != 0 {
		t.Fatal("HTTP before SQL commit")
	}
	o.reopen()
	if err := o.store.drainSyncCleanup(); err != nil {
		t.Fatal(err)
	}
	if len(o.calls("/api/sync/object-receipts")) != 0 {
		t.Fatal("orphan became receipt")
	}
	// The preparation cleanup tracks and removes a file whose SQL ACK failed.
	files, err := filepath.Glob(filepath.Join(filepath.Dir(o.store.path), "ReceiptObjects", "1", "*.bin"))
	if err != nil || len(files) != 0 {
		t.Fatal("preparation file not cleaned", err, files)
	}
}
func TestObjectReceiptPreparedFileCrashAndLateWriterFence(t *testing.T) {
	type seed struct {
		Path      string
		Identity  localIdentity
		Operation receiptOperation
	}
	if output := os.Getenv("JOBSIM_OBJECT_PREPARATION_CRASH_FIXTURE"); output != "" {
		o := cliObjectSetup(t)
		control := o.control()
		parent, err := o.store.loadReceipt(o.parent)
		if err != nil {
			t.Fatal(err)
		}
		id, err := syncNewUUID()
		if err != nil {
			t.Fatal(err)
		}
		d := receiptLocalObject{Object: o.object, Size: o.challenge.Object.Size, Digest: o.challenge.Object.Digest, ContentType: o.challenge.Object.ContentType, Path: receiptObjectPath("1", id), Parent: o.parent, Request: id, Message: o.messageID, Epoch: "1", Binding: control.syncBinding, Content: parent.Content, BodyDigest: parent.Digest}
		body, _ := json.Marshal(map[string]string{"requestId": id, "storeGeneration": o.physical, "revisionEpoch": "1", "qualificationId": o.qualification, "kind": "object_received", "parentReceiptId": o.parent, "challenge": o.challenge.Challenge, "objectId": d.Object, "sizeBytes": d.Size, "objectDigest": d.Digest})
		v := receiptOperation{ID: id, Kind: "object_received", Binding: control.syncBinding, Method: "POST", Route: "/api/sync/object-receipts", Body: string(body), Message: parent.Message, Conversation: parent.Conversation, Content: parent.Content, Digest: parent.Digest, Expires: o.challenge.Expires, State: "pending", Object: &d}
		v.Hash = receiptHash(v)
		current := func() bool { return true }
		if err = o.store.prepareReceiptObject(control, v, current); err != nil {
			t.Fatal(err)
		}
		if err = o.store.persistReceiptObject(control, v, current, bytes.NewReader(o.data)); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(seed{Path: o.store.path, Identity: o.identity, Operation: v})
		if err = os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(92)
	}
	path := filepath.Join(t.TempDir(), "prepared.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestObjectReceiptPreparedFileCrashAndLateWriterFence$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "JOBSIM_OBJECT_PREPARATION_CRASH_FIXTURE="+path, "TMPDIR="+filepath.Dir(path))
	raw, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 92 {
		t.Fatalf("prepared crash: %v %s", err, raw)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v seed
	if json.Unmarshal(encoded, &v) != nil {
		t.Fatal("seed metadata")
	}
	s, err := openStore(v.Path, v.Identity, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if _, err = s.loadReceipt(v.Operation.ID); err == nil {
		t.Fatal("preparation became ACK")
	}
	file, err := s.openReceiptObject(*v.Operation.Object)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	control, _, err := syncLoadControl(s.db, "1")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", v.Operation.Message); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = s.drainSyncCleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.openReceiptObject(*v.Operation.Object); err == nil {
		t.Fatal("prepared holder survived global")
	}
	if err = s.persistReceiptObject(control, v.Operation, func() bool { return true }, bytes.NewReader([]byte("late"))); err == nil {
		t.Fatal("late download recreated path")
	}
	if err = s.stageReceipt(control, v.Operation, nil, func() bool { return true }); err == nil {
		t.Fatal("late SQL created ACK")
	}
	if _, err = s.loadReceipt(v.Operation.ID); err == nil {
		t.Fatal("cancelled preparation promoted")
	}
}
func TestObjectReceiptReplacementSymlinkMissingNeverRebind(t *testing.T) {
	for _, kind := range []string{"modified", "symlink", "missing", "directory"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			o.submitStatus = 503
			_, _ = o.client.receiveReceiptObject(o.parent, "")
			v := o.intent()
			path := filepath.Join(filepath.Dir(o.store.path), filepath.FromSlash(v.Object.Path))
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "modified":
				if err := os.WriteFile(path, bytes.Repeat([]byte("x"), len(o.data)), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				source := filepath.Join(t.TempDir(), "other")
				os.WriteFile(source, o.data, 0600)
				if err := os.Symlink(source, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			o.reopen()
			before := len(o.calls("/api/sync/object-receipts"))
			if _, err := o.client.performReceipt(v.ID, true, false); err == nil {
				t.Fatal("bad local file accepted")
			}
			if len(o.calls("/api/sync/object-receipts")) != before {
				t.Fatal("bad file resubmitted")
			}
			latest := o.intent()
			if latest.Body != v.Body || latest.State != "pending" {
				t.Fatal("intent replaced")
			}
		})
	}
}
func TestObjectReceiptOldBodyContractAndCapabilityCannotUpgrade(t *testing.T) {
	for _, kind := range []string{"old", "missing", "claim", "flag", "build"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			switch kind {
			case "old":
				o.storage.Contract = "native_sqlite_full_v1"
				o.storage.ObjectQualified = false
			case "missing":
				o.storage.Contract = ""
				o.storage.ObjectQualified = false
			case "claim":
				o.storage.Contract = "native_sqlite_full_v1"
			case "flag":
				o.objectFeature = false
			case "build":
				o.storage.Build = strings.Repeat("d", 64)
			}
			if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
				t.Fatal("not refused")
			}
			if len(o.calls("/api/sync/object-receipt-challenges")) != 0 || len(o.calls("/api/media/"+o.object)) != 0 {
				t.Fatal("unqualified fetched object")
			}
		})
	}
}
func TestObjectReceiptParentMustBeRecordedSameQualification(t *testing.T) {
	for _, kind := range []string{"pending", "result", "qualification", "epoch", "physical"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			switch kind {
			case "pending":
				o.store.db.Exec("UPDATE receipt_outbox SET state='pending' WHERE id=?", o.parent)
			case "result":
				o.store.db.Exec("UPDATE receipt_outbox SET result='{}' WHERE id=?", o.parent)
			case "qualification":
				o.storage.Qualification = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			case "epoch":
				o.client.epoch = "2"
			case "physical":
				o.store.db.Exec("UPDATE metadata SET value='dddddddd-dddd-4ddd-8ddd-dddddddddddd' WHERE key='sync-physical-generation'")
			}
			if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
				t.Fatal("wrong parent accepted")
			}
			if len(o.calls("/api/sync/object-receipts")) != 0 {
				t.Fatal("receipt submitted")
			}
		})
	}
}
func TestObjectReceiptChallengeTupleDeadlineAndByteShape(t *testing.T) {
	for _, kind := range []string{"object", "size", "digest", "parent", "epoch", "content", "expired", "proof", "target"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			switch kind {
			case "object":
				o.challenge.Object.ID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			case "size":
				o.challenge.Object.Size = "01"
			case "digest":
				o.challenge.Object.Digest = strings.Repeat("e", 64)
			case "parent":
				o.challenge.Parent = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
			case "epoch":
				o.challenge.Epoch = "2"
			case "content":
				o.challenge.Content = "9007199254740994"
			case "expired":
				o.challenge.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
			case "proof":
				o.challenge.Challenge = "rc1." + strings.TrimPrefix(o.challenge.Challenge, "ro1.")
			case "target":
				yes := true
				o.challenge.Target = &yes
			}
			if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
				t.Fatal("bad challenge accepted")
			}
			if len(o.calls("/api/media/"+o.object)) != 0 || len(o.calls("/api/sync/object-receipts")) != 0 {
				t.Fatal("bad challenge downloaded/submitted")
			}
		})
	}
}
func TestObjectReceiptExpiredMissingFileCanStillQueryHistoricalResult(t *testing.T) {
	t.Run("pending-original-expires", func(t *testing.T) {
		o := cliObjectSetup(t)
		o.challenge.Expires = time.Now().Add(2 * time.Second).Truncate(time.Second).Format(time.RFC3339Nano)
		o.sign()
		o.submitStatus = 503
		if _, err := o.client.receiveReceiptObject(o.parent, ""); err == nil {
			t.Fatal("unknown")
		}
		v := o.intent()
		expires, _ := time.Parse(time.RFC3339Nano, v.Expires)
		time.Sleep(time.Until(expires) + 25*time.Millisecond)
		before := len(o.calls("/api/sync/object-receipts"))
		if _, err := o.client.performReceipt(v.ID, true, false); err == nil {
			t.Fatal("expired original submitted")
		}
		latest := o.intent()
		if latest.State != "frozen" || latest.Body != v.Body || len(o.calls("/api/sync/object-receipts")) != before {
			t.Fatal("expired request changed/replayed")
		}
	})
	o := cliObjectSetup(t)
	if _, err := o.client.receiveReceiptObject(o.parent, ""); err != nil {
		t.Fatal(err)
	}
	v := o.intent()
	path := filepath.Join(filepath.Dir(o.store.path), filepath.FromSlash(v.Object.Path))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	o.storage.Qualified = false
	o.storage.ObjectQualified = false
	if _, err := o.client.performReceipt(v.ID, false, false); err != nil {
		t.Fatal("historical fact should remain queryable", err)
	}
	if _, err := o.store.openReceiptObject(*v.Object); err == nil {
		t.Fatal("missing file restored")
	}
	o.resultStatus = 404
	before := len(o.calls("/api/sync/object-receipts"))
	if _, err := o.client.performReceipt(v.ID, true, false); err == nil {
		t.Fatal("404 not found")
	}
	if len(o.calls("/api/sync/object-receipts")) != before {
		t.Fatal("completed result reconstructed")
	}
}
func TestObjectReceiptGlobalDurableCleanupCrashResume(t *testing.T) {
	t.Run("retained-unknown-and-pending-global", func(t *testing.T) {
		o := cliObjectSetup(t)
		o.submitStatus = 503
		_, _ = o.client.receiveReceiptObject(o.parent, "")
		v := o.intent()
		if err := o.store.restrictSync("SYNC_UNAVAILABLE", false); err != nil {
			t.Fatal(err)
		}
		if err := o.store.drainSyncCleanup(); err != nil {
			t.Fatal(err)
		}
		file, err := o.store.openReceiptObject(*v.Object)
		if err != nil {
			t.Fatal("unknown erased legitimate object", err)
		}
		file.Close()
		local, err := o.store.localHistory(o.conversation)
		if err != nil || len(local) != 1 {
			t.Fatal("ordinary retained copy lost", err)
		}
		tx, err := o.store.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		if err = syncRecordGlobalTx(tx, "1", o.messageID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
		before := len(o.calls("/api/sync/object-receipts"))
		if _, err = o.client.performReceipt(v.ID, true, false); err == nil {
			t.Fatal("global original retried")
		}
		if o.intent().State != "frozen" || len(o.calls("/api/sync/object-receipts")) != before {
			t.Fatal("global did not freeze original")
		}
		if err = o.store.drainSyncCleanup(); err != nil {
			t.Fatal(err)
		}
		if _, err = o.store.openReceiptObject(*v.Object); err == nil {
			t.Fatal("global holder remains")
		}
	})
	o := cliObjectSetup(t)
	source := filepath.Join(t.TempDir(), "original.bin")
	os.WriteFile(source, o.data, 0600)
	if _, err := o.client.receiveReceiptObject(o.parent, source); err != nil {
		t.Fatal(err)
	}
	v := o.intent()
	path := filepath.Join(filepath.Dir(o.store.path), filepath.FromSlash(v.Object.Path))
	tx, err := o.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = syncRecordGlobalTx(tx, "1", o.messageID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("file cleanup must be separately durable")
	}
	o.reopen()
	if err = o.store.drainSyncCleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("managed holder not erased", err)
	}
	if b, err := os.ReadFile(source); err != nil || !bytes.Equal(b, o.data) {
		t.Fatal("user import source erased")
	}
	var n int
	o.store.db.QueryRow("SELECT count(*) FROM receipt_object_cleanup").Scan(&n)
	if n != 0 {
		t.Fatal("cleanup not completed")
	}
	if err = o.store.drainSyncCleanup(); err != nil {
		t.Fatal(err)
	}
}
func TestObjectReceiptDescriptorOriginalHashAndBadSuccess(t *testing.T) {
	for _, kind := range []string{"descriptor", "payload", "result", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			o := cliObjectSetup(t)
			o.submitStatus = 503
			_, _ = o.client.receiveReceiptObject(o.parent, "")
			v := o.intent()
			before := len(o.calls("/api/sync/object-receipts"))
			switch kind {
			case "descriptor":
				o.store.db.Exec("UPDATE receipt_objects SET descriptor_hash='bad'")
			case "payload":
				o.store.db.Exec("UPDATE receipt_outbox SET payload_hash='bad' WHERE id=?", v.ID)
			case "result":
				o.resultStatus = 200
				o.result = json.RawMessage(`{"status":"recorded","objectComplete":true}`)
			case "unknown":
				o.resultStatus = 503
			}
			if _, err := o.client.performReceipt(v.ID, true, false); err == nil {
				t.Fatal("corruption/unknown accepted")
			}
			if len(o.calls("/api/sync/object-receipts")) != before {
				t.Fatal("unverified resubmit")
			}
		})
	}
}
func TestObjectReceiptForwardAndPartialMediaRemainUnsupported(t *testing.T) {
	o := cliObjectSetup(t)
	for _, kind := range []string{"forward", "partial"} {
		t.Run(kind, func(t *testing.T) {
			var m map[string]any
			syncDecode(o.message(), &m)
			m["type"], m["attachmentId"] = "file", o.object
			if kind == "forward" {
				m["forwarded"] = true
			} else {
				m["attachmentId"] = ""
			}
			raw, _ := json.Marshal(m)
			challenge := o.makeChallenge(raw, time.Now().Add(time.Minute).Truncate(time.Second))
			control, _, _ := syncLoadControl(o.store.db, "1")
			if _, err := receiptValidateMessageChallenge(challenge, control.syncBinding, o.qualification, o.messageID, o.permission, time.Now(), false); err == nil {
				t.Fatal("unsupported body acknowledged")
			}
		})
	}
}
