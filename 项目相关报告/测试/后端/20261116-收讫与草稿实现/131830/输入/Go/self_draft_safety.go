package jobsim

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func draftJournalKinds() map[string][]string {
	return map[string][]string{"sync.draft_floor": {"scopeId", "userId", "conversationId", "objectId", "epoch", "version", "contentDigest", "action", "replyState", "operation"}}
}

func validateDraftJournalField(field string, raw any) (bool, error) {
	value, ok := raw.(string)
	valid := false
	switch field {
	case "epoch":
		valid = ok && syncValidationCounter(value, false)
	case "contentDigest":
		_, err := draftDigestBytes(value)
		valid = ok && err == nil
	case "action":
		valid = ok && (value == "set" || value == "clear" || value == "reply_unavailable")
	case "replyState":
		valid = ok && (value == "none" || value == "available" || value == "unavailable")
	case "operation":
		valid = ok && syncValidationID(value)
	default:
		return false, nil
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid typed draft safety field")
	}
	return true, nil
}

func (a *App) protectDraftFloor(ctx context.Context, floor draftFloor) error {
	if err := validateDraftFloor(floor); err != nil {
		return err
	}
	payload := map[string]any{"scopeId": floor.ScopeID, "userId": floor.UserID, "conversationId": floor.ConversationID, "objectId": floor.ObjectID, "epoch": floor.Epoch, "version": floor.Version, "contentDigest": floor.ContentDigest, "action": floor.Action, "replyState": floor.ReplyState, "operation": floor.Operation}
	key := "draft-floor:" + floor.Operation + ":" + floor.Epoch + ":" + floor.Version + ":" + floor.Action
	return a.protect(ctx, key, "intent", "sync.draft_floor", payload)
}

func draftFloorFromJournal(q journalRecord) (draftFloor, error) {
	text := func(name string) string { value, _ := q.Payload[name].(string); return value }
	floor := draftFloor{ScopeID: text("scopeId"), SourceContextID: "personal", UserID: text("userId"), ConversationID: text("conversationId"), ObjectID: text("objectId"), Epoch: text("epoch"), Version: text("version"), ContentDigest: text("contentDigest"), Action: text("action"), ReplyState: text("replyState"), Operation: text("operation")}
	if q.Owner != "sync" || q.Phase != "intent" {
		return floor, fmt.Errorf("draft safety owner or phase differs")
	}
	return floor, validateDraftFloor(floor)
}

// Replaying metadata can deny old content or retain a matching existing cipher;
// it cannot reconstruct a missing text body or grant an old reply qualification.
func ReplayDraftSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if q.Kind != "sync.draft_floor" {
		return false, nil
	}
	floor, err := draftFloorFromJournal(q)
	if err != nil {
		return true, err
	}
	tx, err := business.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	result, err := applyDraftFloorTx(ctx, tx, floor)
	if err != nil {
		return true, err
	}
	if err = applyDraftOperationFloorTx(ctx, tx, result.Floor); err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

func applyDraftOperationFloorTx(ctx context.Context, tx pgx.Tx, floor draftFloor) error {
	_, err := tx.Exec(ctx, `UPDATE sync.draft_operations SET
state=CASE WHEN state='prepared' AND (id::text<>$6 OR $7='reply_unavailable') THEN 'retired' ELSE state END,
intent_ciphertext=CASE WHEN $7='clear' OR (revision_epoch,draft_version)<($4::bigint,$5::bigint) AND id::text<>$6 THEN NULL ELSE intent_ciphertext END,updated_at=now()
WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 AND (revision_epoch,draft_version)<=($4::bigint,$5::bigint)`, floor.ScopeID, floor.UserID, floor.ConversationID, floor.Epoch, floor.Version, floor.Operation, floor.Action)
	return err
}

func (a *App) invalidateSelfDraftReply(ctx context.Context, q draftReplyRevocationRequest) error {
	holder := q.Holder
	if !syncValidationID(holder.HolderID) || !syncValidationID(holder.UserID) || !syncValidationID(holder.OperationID) || !syncValidationID(holder.ConversationID) || len(q.OperationID) < 8 || len(q.OperationID) > 512 {
		return fail(400, "INVALID_DRAFT_REVOCATION", "Original typed holder revocation required")
	}
	identity := a.selfDraftIdentity(User{ID: holder.UserID, RestoreEpoch: holder.RestoreEpoch}, holder.ConversationID)
	if holder.DraftID != identity.ObjectID {
		return fail(409, "DRAFT_SCOPE_CHANGED", "Original holder belongs to another draft")
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = lockDraftContextTx(ctx, tx, identity); err != nil {
		return err
	}
	var floorRaw []byte
	err = tx.QueryRow(ctx, `SELECT floor FROM sync.draft_reply_invalidations WHERE holder_id=$1`, holder.HolderID).Scan(&floorRaw)
	var floor draftFloor
	if err == pgx.ErrNoRows {
		operation, err := scanSelfDraftOperation(tx.QueryRow(ctx, `SELECT `+selfDraftOperationColumns+` FROM sync.draft_operations WHERE id=$1 AND user_id=$2 AND conversation_id=$3`, holder.OperationID, holder.UserID, holder.ConversationID))
		if err == pgx.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		// Only the exact original active/pending holder may affect this draft.
		var reference []byte
		var version, epoch string
		if err = tx.QueryRow(ctx, `SELECT draft_revision_epoch::text,draft_version::text,draft_reply_reference FROM sync.conversation_views WHERE scope_id=$1 AND user_id=$2 AND conversation_id=$3 FOR UPDATE`, identity.ScopeID, holder.UserID, holder.ConversationID).Scan(&epoch, &version, &reference); err != nil {
			return err
		}
		var current draftReplyReference
		_ = json.Unmarshal(reference, &current)
		if current.HolderID != holder.HolderID && (operation.Status != "prepared" || operation.Holder == nil || operation.Holder.HolderID != holder.HolderID) {
			return nil
		}
		floor = operation.Identity
		floor.Action, floor.ReplyState = "reply_unavailable", "unavailable"
		if epoch == floor.Epoch {
			if n, _ := strconv.ParseInt(version, 10, 64); n >= mustDraftCounter(floor.Version) {
				floor.Version, err = draftVersionAfter(version)
				if err != nil {
					return err
				}
			}
		}
		floorRaw, _ = json.Marshal(floor)
		if _, err = tx.Exec(ctx, `INSERT INTO sync.draft_reply_invalidations(holder_id,operation_id,floor,state) VALUES($1,$2,$3,'prepared')`, holder.HolderID, q.OperationID, floorRaw); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if json.Unmarshal(floorRaw, &floor) != nil {
		return fail(503, "DRAFT_FLOOR_CORRUPT", "Original invalidation floor unavailable")
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if err = a.protectDraftFloor(ctx, floor); err != nil {
		return err
	}
	tx, err = a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := applyDraftFloorTx(ctx, tx, floor)
	if err != nil {
		return err
	}
	if err = applyDraftOperationFloorTx(ctx, tx, result.Floor); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.draft_reply_invalidations SET state='applied' WHERE holder_id=$1`, holder.HolderID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mustDraftCounter(value string) int64 {
	n, _ := strconv.ParseInt(value, 10, 64)
	return n
}

// This is an offline restore gate. Independent owned ciphertext is retained;
// mutable reply grants and old unpublished positive preparations are not.
func CancelRestoredDrafts(ctx context.Context, businessURL string, epoch int64) (map[string]int64, error) {
	stats := map[string]int64{"replyHoldersCleared": 0, "pendingDraftsRetired": 0}
	conn, err := operatorConnect(ctx, businessURL)
	if err != nil {
		return stats, err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return stats, err
	}
	defer tx.Rollback(ctx)
	var holders, views, operations, pending bool
	if err = tx.QueryRow(ctx, `SELECT to_regclass('message.draft_reply_holders') IS NOT NULL,to_regclass('sync.draft_floors') IS NOT NULL,to_regclass('sync.draft_operations') IS NOT NULL,to_regclass('sync.draft_pending') IS NOT NULL`).Scan(&holders, &views, &operations, &pending); err != nil {
		return stats, err
	}
	if holders {
		tag, err := tx.Exec(ctx, `UPDATE message.draft_reply_holders SET state='cleared',revocation_operation_id=NULL,updated_at=now() WHERE state<>'cleared'`)
		if err != nil {
			return stats, err
		}
		stats["replyHoldersCleared"] = tag.RowsAffected()
	}
	if views {
		if _, err = tx.Exec(ctx, `UPDATE sync.conversation_views SET draft_reply_reference=NULL,draft_reply_state=CASE WHEN draft_reply_state='none' THEN 'none' ELSE 'unavailable' END WHERE draft_reply_reference IS NOT NULL OR draft_reply_state='available'`); err != nil {
			return stats, err
		}
	}
	if operations {
		tag, err := tx.Exec(ctx, `UPDATE sync.draft_operations SET state='retired',updated_at=now() WHERE state='prepared' AND revision_epoch<$1`, epoch)
		if err != nil {
			return stats, err
		}
		stats["pendingDraftsRetired"] = tag.RowsAffected()
	}
	if pending {
		if _, err = tx.Exec(ctx, `UPDATE sync.draft_pending SET state='retired',reply_reference=NULL,updated_at=now() WHERE draft_epoch<$1 AND state='prepared'`, epoch); err != nil {
			return stats, err
		}
	}
	return stats, tx.Commit(ctx)
}
