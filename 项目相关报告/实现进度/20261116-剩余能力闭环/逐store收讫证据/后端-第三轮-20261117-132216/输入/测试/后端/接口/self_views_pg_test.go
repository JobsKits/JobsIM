package jobsim

import (
	"context"
	"encoding/json"
	"testing"
)

// This reuses SELF_READ's guarded private database, not an additional gate.
// Existing view metadata avoids inventing successful membership/auth RPCs.
func newSelfViewPGTestRealm(t *testing.T) (context.Context, *App, User, string) {
	t.Helper()
	ctx, app, user, conversation, _ := newSelfReadTestRealm(t)
	tx, err := app.DB.Begin(ctx)
	if err != nil {
		t.Fatal("owned personal-view migration transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if migrateSelfViews(ctx, tx, app.Config.DeploymentID) != nil {
		t.Fatal("owned personal-view tables could not be created")
	}
	var tablesReady bool
	if tx.QueryRow(ctx, `SELECT to_regclass('sync.conversation_views') IS NOT NULL AND to_regclass('sync.read_cursors') IS NOT NULL AND to_regclass('sync.read_observations') IS NOT NULL`).Scan(&tablesReady) != nil || !tablesReady {
		t.Fatal("self source tables were not complete before outbox migration")
	}
	if migrateSyncOwner(ctx, tx, "sync", app.Config.DeploymentID) != nil || tx.Commit(ctx) != nil {
		t.Fatal("owned personal metadata outbox migration failed")
	}
	view, found, err := app.selfViewRead(ctx, app.DB, user, conversation)
	if err != nil || found || view.Version != "0" || view.RevisionEpoch != "1" || view.ManualUnreadGeneration != "0" {
		t.Fatal("missing private view did not remain a current-epoch default")
	}
	if selfViewPGTestCount(t, ctx, app, "conversation_views") != 0 || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 0 || selfViewPGTestCount(t, ctx, app, "sync_outbox") != 0 {
		t.Fatal("default private view read wrote a row, operation or outbox event")
	}
	if _, err = app.DB.Exec(ctx, `INSERT INTO sync.conversation_views(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch) VALUES($1,$2,'personal',$3,$4,1)`, view.ID, app.Config.DeploymentID, user.ID, conversation); err != nil {
		t.Fatal("owned existing-view metadata fixture failed")
	}
	if selfViewPGTestCount(t, ctx, app, "sync_outbox") != 0 {
		t.Fatal("version-zero private fixture emitted a false change")
	}
	return ctx, app, user, conversation
}

func selfViewPGTestCount(t *testing.T, ctx context.Context, app *App, table string) int64 {
	t.Helper()
	switch table {
	case "conversation_views", "conversation_view_operations", "sync_outbox":
	default:
		t.Fatal("personal-view test count escaped its owned table whitelist")
	}
	var count int64
	if app.DB.QueryRow(ctx, "SELECT count(*) FROM sync."+table).Scan(&count) != nil {
		t.Fatal("owned personal-view count unavailable")
	}
	return count
}

func selfViewPGTestOutbox(t *testing.T, ctx context.Context, app *App, user User, conversation string, expected int64) {
	t.Helper()
	id := stableID(app.Config.Key, "personal-view:"+app.Config.DeploymentID+":"+user.ID+":"+conversation)
	var count int64
	if app.DB.QueryRow(ctx, `SELECT count(*) FROM sync.sync_outbox WHERE scope_id=$1 AND kind='conversation_view' AND object_id=$2 AND subject_user_id=$3 AND conversation_id=$4 AND target_user_id IS NULL AND state='active' AND metadata=jsonb_build_object('action','update')`, app.Config.DeploymentID, id, user.ID, conversation).Scan(&count) != nil || count != expected || selfViewPGTestCount(t, ctx, app, "sync_outbox") != expected {
		t.Fatal("personal-view outbox did not contain only the committed owner's minimal metadata")
	}
}

func TestSelfViewPGDefaultCASAndOriginalFacts(t *testing.T) {
	ctx, app, user, conversation := newSelfViewPGTestRealm(t)
	yes := true
	unknown := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "0", Changes: selfViewChanges{Hidden: &yes}}
	_, err := app.selfViewApply(ctx, user, conversation, unknown, true)
	selfReadTestError(t, err, 404, "VIEW_OPERATION_NOT_FOUND")
	if selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 0 || selfViewPGTestCount(t, ctx, app, "sync_outbox") != 0 {
		t.Fatal("unknown proof-only view request created an operation or change")
	}
	type result struct {
		patch selfViewPatch
		op    selfViewOperation
		err   error
	}
	start, done := make(chan struct{}), make(chan result, 2)
	for _, changes := range []selfViewChanges{{Hidden: &yes}, {Archived: &yes}} {
		patch := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "0", Changes: changes}
		go func() {
			<-start
			op, err := app.selfViewApply(ctx, user, conversation, patch, false)
			done <- result{patch, op, err}
		}()
	}
	close(start)
	var winner result
	wins := 0
	for range 2 {
		outcome := <-done
		if outcome.err != nil {
			selfReadTestError(t, outcome.err, 409, "VIEW_VERSION_CONFLICT")
			continue
		}
		winner, wins = outcome, wins+1
	}
	if wins != 1 || !winner.op.Current || winner.op.Result.Version != "1" || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 1 {
		t.Fatal("concurrent same-version view mutations did not commit exactly one original fact")
	}
	for _, proof := range []bool{false, true} {
		original, err := app.selfViewApply(ctx, user, conversation, winner.patch, proof)
		if err != nil || !original.Current || original.Result.Version != "1" || original.Result.Hidden != winner.op.Result.Hidden || original.Result.Archived != winner.op.Result.Archived {
			t.Fatal("same original private view request was reapplied or changed")
		}
	}
	changed := winner.patch
	changed.Changes.PinnedRank = json.RawMessage("7")
	_, err = app.selfViewApply(ctx, user, conversation, changed, false)
	selfReadTestError(t, err, 409, "IDEMPOTENCY_CONFLICT")
	later, err := app.selfViewApply(ctx, user, conversation, selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "1", Changes: selfViewChanges{PinnedRank: json.RawMessage("3")}}, false)
	if err != nil || later.Result.Version != "2" || later.Result.PinnedRank == nil || *later.Result.PinnedRank != 3 {
		t.Fatal("later private view did not commit its own version and value")
	}
	for _, proof := range []bool{false, true} {
		original, err := app.selfViewApply(ctx, user, conversation, winner.patch, proof)
		if err != nil || original.Current || original.Result.Version != "1" || original.Result.PinnedRank != nil {
			t.Fatal("old view fact was relabeled as current or overwritten with a later value")
		}
	}
	current, found, err := app.selfViewRead(ctx, app.DB, user, conversation)
	if err != nil || !found || current.Version != "2" || current.PinnedRank == nil || *current.PinnedRank != 3 || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 2 {
		t.Fatal("original fact reconciliation rolled back the newer private view")
	}
	selfViewPGTestOutbox(t, ctx, app, user, conversation, 2)
}

func TestSelfViewPGManualUnreadGenerationPreservesNewIntent(t *testing.T) {
	ctx, app, user, conversation := newSelfViewPGTestRealm(t)
	yes, no := true, false
	mark := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "0", Changes: selfViewChanges{ManualUnread: &yes, ExpectedManualUnreadGeneration: "0"}}
	first, err := app.selfViewApply(ctx, user, conversation, mark, false)
	if err != nil || !first.Result.ManualUnread || first.Result.ManualUnreadGeneration != "1" || first.Result.ManualUnreadMarkedAt == nil {
		t.Fatal("first explicit manual unread intent was not independently versioned")
	}
	ordinary, err := app.selfViewApply(ctx, user, conversation, selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "1", Changes: selfViewChanges{Hidden: &yes}}, false)
	if err != nil || ordinary.Result.Version != "2" || !ordinary.Result.ManualUnread || ordinary.Result.ManualUnreadGeneration != "1" {
		t.Fatal("unrelated view metadata changed or cleared the manual unread intent")
	}
	newMark := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "2", Changes: selfViewChanges{ManualUnread: &yes, ExpectedManualUnreadGeneration: "1"}}
	latest, err := app.selfViewApply(ctx, user, conversation, newMark, false)
	if err != nil || latest.Result.Version != "3" || latest.Result.ManualUnreadGeneration != "2" {
		t.Fatal("new explicit manual unread intent did not advance its independent generation")
	}
	clear := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "3", Changes: selfViewChanges{ManualUnread: &no, ExpectedManualUnreadGeneration: "1"}}
	_, err = app.selfViewApply(ctx, user, conversation, clear, false)
	selfReadTestError(t, err, 409, "MANUAL_UNREAD_CHANGED")
	current, _, err := app.selfViewRead(ctx, app.DB, user, conversation)
	if err != nil || current.Version != "3" || !current.ManualUnread || current.ManualUnreadGeneration != "2" || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 3 {
		t.Fatal("stale manual unread generation cleared a newer marker or persisted an operation")
	}
	clear.Changes.ExpectedManualUnreadGeneration = "2"
	cleared, err := app.selfViewApply(ctx, user, conversation, clear, false)
	if err != nil || cleared.Result.Version != "4" || cleared.Result.ManualUnread || cleared.Result.ManualUnreadGeneration != "3" || cleared.Result.ManualUnreadMarkedAt != nil || cleared.Result.ManualUnreadFrom != nil {
		t.Fatal("current generation did not clear only its own manual unread intent")
	}
	original, err := app.selfViewApply(ctx, user, conversation, mark, true)
	if err != nil || original.Current || !original.Result.ManualUnread || original.Result.ManualUnreadGeneration != "1" {
		t.Fatal("proof lookup lost the old marker fact or revived it as current")
	}
	selfViewPGTestOutbox(t, ctx, app, user, conversation, 4)
}

func TestSelfViewPGEpochZeroRebaseRetainsOriginalFacts(t *testing.T) {
	ctx, app, user, conversation := newSelfViewPGTestRealm(t)
	yes, no := true, false
	patch := selfViewPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "0", Changes: selfViewChanges{Hidden: &yes}}
	first, err := app.selfViewApply(ctx, user, conversation, patch, false)
	if err != nil {
		t.Fatal("owned epoch-one private view fixture failed")
	}
	if _, err = app.DB.Exec(ctx, `UPDATE sync.conversation_views SET row_version=20; UPDATE sync.instance SET restore_epoch=2 WHERE singleton`); err != nil {
		t.Fatal("owned private view epoch transition fixture failed")
	}
	currentUser := user
	currentUser.RestoreEpoch = "2"
	current, found, err := app.selfViewRead(ctx, app.DB, currentUser, conversation)
	if err != nil || !found || current.Version != "0" || current.RevisionEpoch != "2" || !current.Hidden || current.ManualUnreadGeneration != "0" || current.ID != first.Result.ID {
		t.Fatal("epoch-one version twenty blocked the stable new-epoch version-zero view")
	}
	before := selfViewPGTestCount(t, ctx, app, "sync_outbox")
	for _, proof := range []bool{false, true} {
		original, err := app.selfViewApply(ctx, currentUser, conversation, patch, proof)
		if err != nil || original.Current || original.Result.RevisionEpoch != "1" || original.Result.Version != "1" {
			t.Fatal("old successful view fact was lost, relabeled or applied into the restored epoch")
		}
	}
	missing := patch
	missing.RequestID = newID()
	_, err = app.selfViewApply(ctx, currentUser, conversation, missing, false)
	selfReadTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
	_, err = app.selfViewApply(ctx, currentUser, conversation, missing, true)
	selfReadTestError(t, err, 404, "VIEW_OPERATION_NOT_FOUND")
	if selfViewPGTestCount(t, ctx, app, "sync_outbox") != before || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 1 {
		t.Fatal("unknown old epoch or original lookup emitted a new mutation")
	}
	missing.RevisionEpoch, missing.ExpectedVersion = "2", "0"
	missing.Changes.Hidden = &no
	updated, err := app.selfViewApply(ctx, currentUser, conversation, missing, false)
	if err != nil || !updated.Current || updated.Result.RevisionEpoch != "2" || updated.Result.Version != "1" || updated.Result.Hidden || updated.Result.ID != first.Result.ID {
		t.Fatal("first restored-epoch view write did not start at one while retaining identity")
	}
	_, err = app.selfViewApply(ctx, user, conversation, patch, false)
	selfReadTestError(t, err, 409, "RESTORE_EPOCH_CHANGED")
	var emitted bool
	if app.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sync.sync_outbox WHERE kind='conversation_view' AND object_id=$1 AND restore_epoch=2 AND revision_epoch=2 AND subject_user_id=$2 AND conversation_id=$3)`, updated.Result.ID, user.ID, conversation).Scan(&emitted) != nil || !emitted || selfViewPGTestCount(t, ctx, app, "conversation_view_operations") != 2 {
		t.Fatal("new epoch view fact did not produce its own source-epoch metadata")
	}
}
