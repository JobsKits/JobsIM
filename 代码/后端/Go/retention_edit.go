package jobsim

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// An edit carries the first send's target snapshot and absolute upper bounds.
// The source EX fence excludes a concurrent completion while this is prepared.
func (a *App) prepareRetentionEdit(ctx context.Context, m Message) (*retentionPrepared, error) {
	var raw, manifestRaw []byte
	var generation, content string
	var first *time.Time
	var state string
	var prepared retentionPrepared
	err := a.DB.QueryRow(ctx, `SELECT policy,policy_version::text,manifest,generation::text,content_version::text,receipt_complete_at,state FROM message.retention_assignments WHERE scope_id=$1 AND message_id=$2`, a.Config.DeploymentID, m.ID).Scan(&raw, &prepared.PolicyVersion, &manifestRaw, &generation, &content, &first, &state)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if json.Unmarshal(raw, &prepared.Policy) != nil || json.Unmarshal(manifestRaw, &prepared.Manifest) != nil || !validReceiptManifest(prepared.Manifest) {
		return nil, fail(503, "RETENTION_ASSIGNMENT_CORRUPT", "Original edit boundaries unavailable")
	}
	if m.ContentUnavailable || m.Deleted || content != m.ContentVersion || state == "cloud_deleted" || state == "global_deleted" {
		return nil, fail(409, "RETENTION_EDIT_CLOSED", "Cloud content is no longer editable")
	}
	if prepared.Policy.Mode == "after_receipt" && (first != nil || state != "waiting") {
		return nil, fail(409, "RETENTION_EDIT_SCHEDULED", "A completed or scheduled receipt range cannot be edited")
	}
	if !prepared.Manifest.Expires.After(time.Now()) {
		return nil, fail(409, "RETENTION_EDIT_EXPIRED", "Editing cannot extend the original deadline")
	}
	gn, e := strconv.ParseInt(generation, 10, 64)
	if e != nil || gn == 9223372036854775807 {
		return nil, fail(409, "RETENTION_GENERATION_EXHAUSTED", "Original generation cannot advance")
	}
	cv, e := strconv.ParseInt(content, 10, 64)
	if e != nil || cv == 9223372036854775807 {
		return nil, fail(409, "RETENTION_GENERATION_EXHAUSTED", "Original content version cannot advance")
	}
	prepared.Manifest.Generation, prepared.Manifest.Content = str(gn+1), str(cv+1)
	prepared.Manifest.Digest = ""
	prepared.Manifest.CompleteAt = nil
	prepared.Manifest.Targets = []receiptDeliveryTarget{}
	if a.Config.NATSURL != "" {
		var frozen receiptDeliveryManifest
		if e = a.rpc(ctx, "sync", "/internal/receipts/freeze", prepared.Manifest, &frozen); e != nil {
			return nil, e
		}
		prepared.Manifest = frozen
	} else {
		var previous receiptDeliveryManifest
		_ = json.Unmarshal(manifestRaw, &previous)
		prepared.Manifest.Targets = previous.Targets
		prepared.Manifest.Digest = receiptManifestDigest(prepared.Manifest)
	}
	if !validReceiptManifest(prepared.Manifest) {
		return nil, fail(503, "DELIVERY_MANIFEST_INVALID", "An edit must preserve the original target snapshot")
	}
	return &prepared, nil
}
func (a *App) persistRetentionEdit(ctx context.Context, tx pgx.Tx, id string, p *retentionPrepared) error {
	if p == nil {
		return nil
	}
	raw, _ := json.Marshal(p.Manifest)
	changed, err := tx.Exec(ctx, `UPDATE message.retention_assignments SET content_version=$3,generation=$4,manifest=$5,manifest_digest=$6,receipt_complete_at=NULL,object_complete_at=NULL,state='waiting',updated_at=now() WHERE scope_id=$1 AND message_id=$2 AND generation=$4::bigint-1 AND (receipt_complete_at IS NULL OR policy->>'mode'='cloud_history')`, a.Config.DeploymentID, id, p.Manifest.Content, p.Manifest.Generation, raw, p.Manifest.Digest)
	if err != nil {
		return err
	}
	if changed.RowsAffected() != 1 {
		return fail(409, "RETENTION_EDIT_CHANGED", "The original receipt range changed")
	}
	return nil
}
