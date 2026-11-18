package jobsim

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func draftFloorTestIdentity() (*App, draftFloor) {
	a := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x78}, 32)}, Role: "sync"}
	floor := draftFloor{ScopeID: a.Config.DeploymentID, SourceContextID: "personal", UserID: newID(), ConversationID: newID(), Epoch: "1", Version: "5", Action: "set", ReplyState: "none", Operation: newID()}
	floor.ObjectID = stableID(a.Config.Key, "personal-view:"+floor.ScopeID+":"+floor.UserID+":"+floor.ConversationID)
	floor.ContentDigest, _ = a.draftOwnedDigest(floor, "NON_SECRET_OWNED_DRAFT_A")
	return a, floor
}

func draftFloorTestError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var rejection *apiError
	if !errors.As(err, &rejection) || rejection.Status != status || rejection.Code != code {
		t.Fatal("draft metadata did not produce its required rejection; private values suppressed")
	}
}

func TestDraftOwnedCipherBindingSurvivesLogicalRebase(t *testing.T) {
	a, identity := draftFloorTestIdentity()
	ciphertext, digest, err := a.sealDraftOwnedText(identity, "1", "5", "NON_SECRET_OWNED_DRAFT_A")
	if err != nil {
		t.Fatal("local non-secret owned-text cipher fixture failed")
	}
	rebased := identity
	rebased.Epoch, rebased.Version, rebased.Operation = "2", "0", newID()
	opened, err := a.openDraftOwnedText(rebased, "1", "5", ciphertext, digest)
	if err != nil || opened != "NON_SECRET_OWNED_DRAFT_A" {
		t.Fatal("logical epoch/version rebase broke the independent original cipher AAD tuple")
	}
	_, err = a.openDraftOwnedText(rebased, "2", "1", ciphertext, digest)
	draftFloorTestError(t, err, 409, "DRAFT_CONTENT_UNAVAILABLE")
	other := rebased
	other.UserID = newID()
	_, err = a.openDraftOwnedText(other, "1", "5", ciphertext, digest)
	draftFloorTestError(t, err, 409, "DRAFT_CONTENT_UNAVAILABLE")
	again, _ := a.draftOwnedDigest(rebased, "NON_SECRET_OWNED_DRAFT_A")
	if again != digest {
		t.Fatal("logical revision or reply intent leaked into the owned-text HMAC identity")
	}
}

func TestDraftFloorDenialCannotBeDowngradedAtSameTuple(t *testing.T) {
	_, positive := draftFloorTestIdentity()
	denial := positive
	denial.Action, denial.ReplyState, denial.Operation = "reply_unavailable", "unavailable", newID()
	winner, advanced, err := chooseDraftFloor(positive, denial)
	if err != nil || !advanced || winner != denial {
		t.Fatal("reply denial did not dominate an equal-tuple positive intent")
	}
	winner, advanced, err = chooseDraftFloor(denial, positive)
	if err != nil || advanced || winner != denial {
		t.Fatal("late positive stage overwrote an equal-tuple reply denial")
	}
	clear := positive
	clear.Action, clear.ReplyState, clear.Operation = "clear", "none", newID()
	winner, _, err = chooseDraftFloor(clear, positive)
	if err != nil || winner != clear {
		t.Fatal("late positive stage overwrote a known clear floor")
	}
	conflict := positive
	conflict.Operation = newID()
	_, _, err = chooseDraftFloor(positive, conflict)
	draftFloorTestError(t, err, 409, "DRAFT_FLOOR_CONFLICT")
	first, second := positive, positive
	first.Epoch, first.Version = "1", "9223372036854775807"
	second.Epoch, second.Version = "2", "9007199254740993"
	comparison, err := compareDraftFloorTuple(first, second)
	if err != nil || comparison >= 0 {
		t.Fatal("draft floor comparison lost exact epoch/version ordering")
	}
}

func TestDraftReplyFloorKeepsOnlyMatchingOwnedCipher(t *testing.T) {
	a, floor := draftFloorTestIdentity()
	ciphertext, digest, err := a.sealDraftOwnedText(floor, "1", "5", "NON_SECRET_OWNED_DRAFT_A")
	if err != nil {
		t.Fatal("local owned cipher fixture failed")
	}
	decoded, _ := hex.DecodeString(digest)
	epoch, version := int64(1), int64(5)
	stored := draftStoredContent{Epoch: "1", Version: "5", Operation: newID(), ContentState: "owned", Digest: decoded, Ciphertext: ciphertext, CipherEpoch: &epoch, CipherVersion: &version, ReplyState: "available"}
	floor.Version, floor.Action, floor.ReplyState = "6", "reply_unavailable", "unavailable"
	plan, err := planDraftFloor(floor, stored)
	if err != nil || !plan.Apply || !plan.KeepCipher || plan.KeepReply || plan.ContentState != "owned" || plan.ReplyState != "unavailable" {
		t.Fatal("reference-only invalidation erased the author's matching independent ciphertext")
	}
	floor.ContentDigest, _ = a.draftOwnedDigest(floor, "NON_SECRET_NEWER_OWNED_DRAFT_B")
	plan, err = planDraftFloor(floor, stored)
	if err != nil || plan.KeepCipher || plan.ContentState != "unavailable" {
		t.Fatal("missing newer owned text fell back to the older saved body")
	}
	stored.Epoch, stored.Version = "2", "0"
	floor.Action, floor.ReplyState = "clear", "none"
	plan, err = planDraftFloor(floor, stored)
	if err != nil || !plan.Apply || plan.KeepCipher || plan.ContentState != "empty" || plan.Epoch != "2" || plan.Version != "0" {
		t.Fatal("logical rebase hid the old cipher from its historical clear floor")
	}
	stored.Version = "1"
	plan, err = planDraftFloor(floor, stored)
	if err != nil || plan.Apply {
		t.Fatal("historical floor overwrote a later explicit new-epoch draft")
	}
}

// Future execution creates only this new tiny sync-owned database. It does not
// append journal records, invoke holders, authenticate accounts or call HTTP.
func newDraftFloorTestRealm(t *testing.T) (context.Context, *pgx.Conn, *App, draftFloor) {
	t.Helper()
	if os.Getenv("JOBS_IM_DRAFT_FLOOR_TEST") != "1" {
		t.Skip("explicit future isolated draft-floor test window required")
	}
	address, err := url.Parse(os.Getenv("JOBS_IM_DRAFT_FLOOR_REALM_URL"))
	if err != nil || address.Scheme != "https" || address.User != nil || address.Port() == "" || address.Port() == "44443" || net.ParseIP(address.Hostname()) == nil || !net.ParseIP(address.Hostname()).IsLoopback() {
		t.Fatal("future loopback TLS realm distinct from frozen 44443 required")
	}
	dsn := os.Getenv("JOBS_IM_DRAFT_FLOOR_ADMIN_URL")
	configuration, err := pgx.ParseConfig(dsn)
	if dsn == "" || err != nil || configuration.Database != "postgres" {
		t.Fatal("explicit PostgreSQL maintenance connection required; details redacted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, configuration)
	if err != nil {
		t.Fatal("owned draft-floor creation connection unavailable")
	}
	name := "jobsim_draft_floor_" + strings.ReplaceAll(newID(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted+" TEMPLATE template0"); err != nil {
		admin.Close(ctx)
		t.Fatal("owned draft-floor database could not be created")
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+quoted); err != nil {
			t.Error("owned draft-floor cleanup failed; no shared connection was terminated")
		}
		admin.Close(cleanup)
	})
	child := configuration.Copy()
	child.Database = name
	child.RuntimeParams["application_name"] = "jobsim-draft-floor-metadata-test"
	db, err := pgx.ConnectConfig(ctx, child)
	if err != nil {
		t.Fatal("owned draft-floor database unavailable")
	}
	t.Cleanup(func() { db.Close(context.Background()) })
	if _, err = db.Exec(ctx, "CREATE SCHEMA sync;CREATE TABLE sync.instance(singleton boolean PRIMARY KEY CHECK(singleton),deployment_id text NOT NULL,restore_epoch bigint NOT NULL)"); err != nil {
		t.Fatal("owned draft-floor instance schema failed")
	}
	a, floor := draftFloorTestIdentity()
	if _, err = db.Exec(ctx, "INSERT INTO sync.instance VALUES(true,$1,1)", floor.ScopeID); err != nil {
		t.Fatal("owned draft-floor scope fixture failed")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal("owned draft-floor migration transaction unavailable")
	}
	if migrateSelfViews(ctx, tx, floor.ScopeID) != nil || migrateSelfDraftFloor(ctx, tx, floor.ScopeID) != nil || tx.Commit(ctx) != nil {
		tx.Rollback(ctx)
		t.Fatal("owned draft-floor migration failed")
	}
	return ctx, db, a, floor
}

func draftFloorTestTx(t *testing.T, ctx context.Context, db *pgx.Conn, action func(pgx.Tx)) {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal("owned draft-floor transaction unavailable")
	}
	defer tx.Rollback(ctx)
	action(tx)
	if tx.Commit(ctx) != nil {
		t.Fatal("owned draft-floor transaction did not commit")
	}
}

func TestDraftFloorAbsentViewUnknownStageAndLateInsert(t *testing.T) {
	ctx, db, a, identity := newDraftFloorTestRealm(t)
	identity.Version = "1"
	ciphertext, digest, err := a.sealDraftOwnedText(identity, "1", "1", "NON_SECRET_OWNED_DRAFT_A")
	if err != nil {
		t.Fatal("owned staged cipher fixture failed")
	}
	identity.ContentDigest = digest
	pending := draftPending{Floor: identity, RequestDigest: strings.Repeat("12", 32), OwnedCiphertext: ciphertext, CipherEpoch: "1", CipherVersion: "1"}
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		staged, err := stageDraftPendingTx(ctx, tx, pending)
		if err != nil || staged.State != "prepared" || !bytes.Equal(staged.OwnedCiphertext, ciphertext) {
			t.Fatal("unknown floor discarded its unresolved original stage")
		}
		allowed, err := draftPendingAllowedTx(ctx, tx, identity)
		if err != nil || allowed {
			t.Fatal("missing independent floor was mistaken for permission to publish")
		}
	})
	clear := identity
	clear.Action, clear.ReplyState, clear.Operation = "clear", "none", newID()
	clear.ContentDigest, _ = a.draftOwnedDigest(clear, "")
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		result, err := applyDraftFloorTx(ctx, tx, clear)
		if err != nil || !result.Advanced || result.ViewPresent || result.PendingRetired != 1 {
			t.Fatal("clear did not persist independently and retire an earlier unknown stage")
		}
	})
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		original, err := stageDraftPendingTx(ctx, tx, pending)
		if err != nil || original.State != "retired" || len(original.OwnedCiphertext) != 0 {
			t.Fatal("original retired stage regained bytes on retry")
		}
		late := pending
		late.Floor.Operation = newID()
		staged, err := stageDraftPendingTx(ctx, tx, late)
		if err != nil || staged.State != "retired" || len(staged.OwnedCiphertext) != 0 {
			t.Fatal("known clear permitted a late equal-tuple ciphertext insert")
		}
	})
	var views int
	if db.QueryRow(ctx, "SELECT count(*) FROM sync.conversation_views").Scan(&views) != nil || views != 0 {
		t.Fatal("floor or stage invented a missing private conversation view")
	}
	if _, err = db.Exec(ctx, "UPDATE sync.instance SET restore_epoch=2 WHERE singleton"); err != nil {
		t.Fatal("owned draft-floor epoch fixture failed")
	}
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		old := pending
		old.Floor.Operation, old.Floor.Version = newID(), "2"
		_, err := stageDraftPendingTx(ctx, tx, old)
		draftFloorTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
		allowed, err := draftPendingAllowedTx(ctx, tx, pending.Floor)
		if err != nil || allowed {
			t.Fatal("old-epoch stage retained positive publish authority after restore")
		}
	})
}

func TestDraftFloorPreservesOwnedBytesAndRejectsOldBackupText(t *testing.T) {
	ctx, db, a, floor := newDraftFloorTestRealm(t)
	ciphertext, digest, err := a.sealDraftOwnedText(floor, "1", "5", "NON_SECRET_OWNED_DRAFT_A")
	if err != nil {
		t.Fatal("owned cipher fixture failed")
	}
	digestBytes, _ := hex.DecodeString(digest)
	if _, err = db.Exec(ctx, `INSERT INTO sync.conversation_views(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch,draft_revision_epoch,draft_version,draft_operation_id,draft_content_state,draft_owned_ciphertext,draft_owned_digest,draft_cipher_epoch,draft_cipher_version,draft_reply_state)
VALUES($1,$2,'personal',$3,$4,1,1,5,$5,'owned',$6,$7,1,5,'available')`, floor.ObjectID, floor.ScopeID, floor.UserID, floor.ConversationID, newID(), ciphertext, digestBytes); err != nil {
		t.Fatal("owned independent content fixture failed")
	}
	floor.Version, floor.Action, floor.ReplyState, floor.Operation = "6", "reply_unavailable", "unavailable", newID()
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := applyDraftFloorTx(ctx, tx, floor); err != nil {
			t.Fatal("owned reference invalidation floor failed")
		}
	})
	var kept []byte
	var cipherEpoch, cipherVersion, state, reply string
	if db.QueryRow(ctx, "SELECT draft_owned_ciphertext,draft_cipher_epoch::text,draft_cipher_version::text,draft_content_state,draft_reply_state FROM sync.conversation_views WHERE id=$1", floor.ObjectID).Scan(&kept, &cipherEpoch, &cipherVersion, &state, &reply) != nil || !bytes.Equal(kept, ciphertext) || cipherEpoch != "1" || cipherVersion != "5" || state != "owned" || reply != "unavailable" {
		t.Fatal("reply-only invalidation changed owned ciphertext or its original AAD tuple")
	}
	floor.Version, floor.Operation = "7", newID()
	floor.ContentDigest, _ = a.draftOwnedDigest(floor, "NON_SECRET_NEWER_DRAFT_NOT_IN_BACKUP")
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := applyDraftFloorTx(ctx, tx, floor); err != nil {
			t.Fatal("newer unavailable-content floor failed")
		}
	})
	var missing bool
	if db.QueryRow(ctx, "SELECT draft_owned_ciphertext IS NULL AND draft_cipher_epoch IS NULL AND draft_cipher_version IS NULL,draft_content_state FROM sync.conversation_views WHERE id=$1", floor.ObjectID).Scan(&missing, &state) != nil || !missing || state != "unavailable" {
		t.Fatal("old backup owned text survived a different newer owned-content digest")
	}
	floor.Version, floor.Action, floor.ReplyState, floor.Operation = "8", "clear", "none", newID()
	floor.ContentDigest, _ = a.draftOwnedDigest(floor, "")
	if _, err = db.Exec(ctx, "UPDATE sync.conversation_views SET draft_revision_epoch=2,draft_version=0,draft_content_state='owned',draft_owned_ciphertext=$2,draft_owned_digest=$3,draft_cipher_epoch=1,draft_cipher_version=5 WHERE id=$1", floor.ObjectID, ciphertext, digestBytes); err != nil {
		t.Fatal("owned logical rebase fixture failed")
	}
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := applyDraftFloorTx(ctx, tx, floor); err != nil {
			t.Fatal("historical clear did not apply across the logical-only rebase")
		}
	})
	var epoch, version string
	if db.QueryRow(ctx, "SELECT draft_revision_epoch::text,draft_version::text,draft_owned_ciphertext IS NULL,draft_content_state FROM sync.conversation_views WHERE id=$1", floor.ObjectID).Scan(&epoch, &version, &missing, &state) != nil || epoch != "2" || version != "0" || !missing || state != "empty" {
		t.Fatal("logical rebase masked a historical erasure or was downgraded by its floor")
	}
}

func TestDraftPendingExactFloorRequiredAndImmutableMetadata(t *testing.T) {
	ctx, db, a, floor := newDraftFloorTestRealm(t)
	floor.Version = "1"
	ciphertext, digest, err := a.sealDraftOwnedText(floor, "1", "1", "NON_SECRET_OWNED_DRAFT_A")
	if err != nil {
		t.Fatal("owned pending cipher fixture failed")
	}
	floor.ContentDigest = digest
	pending := draftPending{Floor: floor, RequestDigest: strings.Repeat("23", 32), OwnedCiphertext: ciphertext, CipherEpoch: "1", CipherVersion: "1"}
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := stageDraftPendingTx(ctx, tx, pending); err != nil {
			t.Fatal("owned original pending stage failed")
		}
		changed := pending
		changed.RequestDigest = strings.Repeat("34", 32)
		_, err := stageDraftPendingTx(ctx, tx, changed)
		draftFloorTestError(t, err, 409, "DRAFT_OPERATION_CONFLICT")
		if _, err = applyDraftFloorTx(ctx, tx, floor); err != nil {
			t.Fatal("original positive content floor failed")
		}
		allowed, err := draftPendingAllowedTx(ctx, tx, floor)
		if err != nil || !allowed {
			t.Fatal("exact original confirmed stage was not distinguishable from unknown authority")
		}
		// Simulate the parent's verified positive commit in the same owner tx.
		digestBytes, _ := hex.DecodeString(digest)
		if _, err = tx.Exec(ctx, `INSERT INTO sync.conversation_views(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch,draft_revision_epoch,draft_version,draft_operation_id,draft_content_state,draft_owned_ciphertext,draft_owned_digest,draft_cipher_epoch,draft_cipher_version)
VALUES($1,$2,'personal',$3,$4,1,1,1,$5,'owned',$6,$7,1,1)`, floor.ObjectID, floor.ScopeID, floor.UserID, floor.ConversationID, floor.Operation, ciphertext, digestBytes); err != nil {
			t.Fatal("owned positive publication fixture failed")
		}
		if err = finishDraftPendingTx(ctx, tx, floor); err != nil {
			t.Fatal("original staged bytes were not retired after the matching parent commit")
		}
	})
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		original, err := stageDraftPendingTx(ctx, tx, pending)
		if err != nil || original.State != "applied" || len(original.OwnedCiphertext) != 0 {
			t.Fatal("applied original nonce was overwritten or recreated with new staged bytes")
		}
	})
}

func TestDraftReferenceRetirementPreservesUnpublishedOwnedText(t *testing.T) {
	ctx, db, a, floor := newDraftFloorTestRealm(t)
	floor.Version = "1"
	ciphertext, digest, err := a.sealDraftOwnedText(floor, "1", "1", "NON_SECRET_UNPUBLISHED_OWNED_TEXT")
	if err != nil {
		t.Fatal("owned pending fixture failed")
	}
	floor.ContentDigest = digest
	pending := draftPending{Floor: floor, RequestDigest: strings.Repeat("45", 32), OwnedCiphertext: ciphertext, CipherEpoch: "1", CipherVersion: "1"}
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := stageDraftPendingTx(ctx, tx, pending); err != nil {
			t.Fatal("original owned pending text could not be staged")
		}
	})
	denial := floor
	denial.Action, denial.ReplyState, denial.Operation = "reply_unavailable", "unavailable", newID()
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := applyDraftFloorTx(ctx, tx, denial); err != nil {
			t.Fatal("reference-only denial failed")
		}
		original, found, err := loadDraftPendingTx(ctx, tx, floor)
		if err != nil || !found || original.State != "retired" || !bytes.Equal(original.OwnedCiphertext, ciphertext) || original.CipherEpoch != "1" || original.CipherVersion != "1" || original.Reply != nil {
			t.Fatal("retiring a source reference erased the author's unpublished independent text")
		}
		allowed, err := draftPendingAllowedTx(ctx, tx, floor)
		if err != nil || allowed {
			t.Fatal("retained owned text restored publication authority to the old intent")
		}
	})
	clear := denial
	clear.Version, clear.Action, clear.ReplyState, clear.Operation = "2", "clear", "none", newID()
	clear.ContentDigest, _ = a.draftOwnedDigest(clear, "")
	draftFloorTestTx(t, ctx, db, func(tx pgx.Tx) {
		if _, err := applyDraftFloorTx(ctx, tx, clear); err != nil {
			t.Fatal("explicit author clear failed")
		}
		original, _, err := loadDraftPendingTx(ctx, tx, floor)
		if err != nil || len(original.OwnedCiphertext) != 0 || original.CipherEpoch != "" || original.CipherVersion != "" {
			t.Fatal("explicit clear did not erase the retained unpublished owned text")
		}
	})
}
