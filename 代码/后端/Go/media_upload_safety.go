package jobsim

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func mediaUploadOperation(request string) string {
	return "media-upload:" + syncDigest([]byte(request))
}

func mediaUploadSafety(d mediaUploadRecord) map[string]any {
	return map[string]any{"requestKey": d.RequestID, "userId": d.UserID, "deviceId": d.DeviceID, "familyId": d.FamilyID, "restoreEpoch": d.Epoch, "securityVersion": d.Security, "sourceGeneration": d.SourceGeneration, "objectId": d.ObjectID, "uploadDigest": hex.EncodeToString(d.Binding)}
}

func mediaUploadJournalKinds() map[string][]string {
	return map[string][]string{"media.upload": {"requestKey", "userId", "deviceId", "familyId", "restoreEpoch", "securityVersion", "sourceGeneration", "objectId", "uploadDigest"}}
}

func validateMediaUploadJournalField(field string, raw any) (bool, error) {
	if field != "uploadDigest" && field != "sourceGeneration" {
		return false, nil
	}
	value, ok := raw.(string)
	valid := ok && syncValidationDigest(value)
	if field == "sourceGeneration" {
		n, err := strconv.ParseInt(value, 10, 64)
		valid = ok && err == nil && n > 0 && str(n) == value
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid minimal media-upload safety field")
	}
	return true, nil
}

// A restored snapshot may predate the local request. Its durable intent then
// creates only a denial responsibility with the exact UUID, never an object.
func ReplayMediaUploadSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if q.Kind != "media.upload" {
		return false, nil
	}
	if q.Owner != "media" {
		return true, fmt.Errorf("media upload safety owner differs")
	}
	if q.Phase != "intent" {
		return true, nil
	}
	text := func(k string) string { v, _ := q.Payload[k].(string); return v }
	bound, err := hex.DecodeString(text("uploadDigest"))
	if err != nil || len(bound) != 32 {
		return true, fmt.Errorf("invalid media upload safety binding")
	}
	tx, err := business.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, mediaUploadMigration); err != nil {
		return true, err
	}
	var scope string
	if err = tx.QueryRow(ctx, `SELECT deployment_id FROM media.instance WHERE singleton`).Scan(&scope); err != nil {
		return true, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO media.upload_requests(scope_id,request_id,user_id,device_id,family_id,restore_epoch,security_version,source_generation,object_id,name,content_type,size_bytes,sha256,request_digest,state)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'','application/octet-stream',0,decode('','hex'),$10,'retired') ON CONFLICT DO NOTHING`, scope, text("requestKey"), text("userId"), text("deviceId"), text("familyId"), text("restoreEpoch"), text("securityVersion"), text("sourceGeneration"), text("objectId"), bound)
	if err != nil {
		return true, err
	}
	d, err := scanMediaUpload(tx.QueryRow(ctx, `SELECT `+mediaUploadColumns+` FROM media.upload_requests WHERE scope_id=$1 AND request_id=$2`, scope, text("requestKey")))
	if err != nil {
		return true, err
	}
	if d.UserID != text("userId") || d.DeviceID != text("deviceId") || d.FamilyID != text("familyId") || d.Epoch != text("restoreEpoch") || d.Security != text("securityVersion") || d.SourceGeneration != text("sourceGeneration") || d.ObjectID != text("objectId") || !bytes.Equal(d.Binding, bound) {
		return true, fmt.Errorf("restored media upload responsibility differs")
	}
	return true, tx.Commit(ctx)
}
