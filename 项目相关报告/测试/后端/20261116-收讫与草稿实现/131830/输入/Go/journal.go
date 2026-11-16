package jobsim

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Journal records never contain passwords, token material or message bodies.
type journalRecord struct {
	Owner        string         `json:"owner"`
	OperationKey string         `json:"operationKey"`
	Phase        string         `json:"phase"`
	Kind         string         `json:"kind"`
	Payload      map[string]any `json:"payload"`
}
type journalState struct {
	RestoreEpoch int64 `json:"restoreEpoch"`
}

func (a *App) epoch(ctx context.Context) (int64, error) {
	var v journalState
	if e := a.rpc(ctx, "journal", "/internal/state", map[string]string{}, &v); e != nil {
		return 0, e
	}
	if v.RestoreEpoch < 1 {
		return 0, fail(503, "JOURNAL_UNVERIFIED", "Invalid independent restore epoch")
	}
	return v.RestoreEpoch, nil
}
func (a *App) protect(ctx context.Context, key, phase, kind string, payload map[string]any) error {
	var out map[string]any
	return a.rpc(ctx, "journal", "/internal/append", journalRecord{a.Role, key, phase, kind, payload}, &out)
}
func (a *App) journalRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/auth-retirement", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID     string `json:"userId"`
			RequestKey string `json:"requestKey"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		if r.Header.Get("X-JobsIM-Caller") != "identity" || !validObjectID(q.UserID) {
			return fail(403, "JOURNAL_OWNER_MISMATCH", "Authentication owner required")
		}
		if e := validateJournalField("requestKey", q.RequestKey); e != nil {
			return e
		}
		var replacement string
		e := a.DB.QueryRow(r.Context(), `SELECT payload->>'replacementKey' FROM journal.entries WHERE owner='identity' AND operation_key=$1 AND phase='intent' AND kind='auth.retire_authentication'`, authenticationRetirementOperation(q.UserID, q.RequestKey)).Scan(&replacement)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		respond(w, 200, map[string]any{"retired": e == nil, "replacementKey": replacement})
		return nil
	}))
	h("POST /internal/state", internal(func(w http.ResponseWriter, r *http.Request) error {
		var epoch int64
		if e := a.DB.QueryRow(r.Context(), `SELECT restore_epoch FROM journal.restore_epochs WHERE singleton`).Scan(&epoch); e != nil {
			return e
		}
		respond(w, 200, journalState{epoch})
		return nil
	}))
	h("POST /internal/append", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q journalRecord
		if e := decode(r, &q); e != nil {
			return e
		}
		allowed := map[string][]string{"auth.reserve_username": {"userId", "username"}, "auth.invalidate_user": {"userId", "generation", "securityVersion"}, "auth.account_state": {"userId", "state", "stateBefore", "banUntil", "securityVersion", "version"},
			"auth.mfa_step":      {"userId", "factorId", "step"},
			"auth.revoke_device": {"userId", "deviceId"}, "auth.recovery_batch": {"userId", "batchId", "generation"}, "directory.block": {"userId", "peerId"}, "directory.unblock": {"userId", "peerId"}, "conversation.leave": {"conversationId", "userId"}, "message.retract": {"messageId"}}
		allowed["message.export_holder"] = []string{"messageId", "userId", "jobId", "artifactId", "buildGeneration", "restoreEpoch", "sourceVersion", "expiresAt"}
		allowed["media.export_holder"] = []string{"objectId", "messageId", "userId", "jobId", "artifactId", "buildGeneration", "restoreEpoch", "expiresAt"}
		allowed["governance.export_fence"] = []string{"jobId", "artifactId", "restoreEpoch", "buildGeneration", "state"}
		allowed["conversation.owner_unavailable"] = []string{"conversationId", "userId"}
		allowed["message.content_floor"] = []string{"messageId", "version"}
		allowed["auth.retire_authentication"] = []string{"userId", "requestKey", "replacementKey"}
		if _, current := q.Payload["membershipGeneration"]; q.Kind == "conversation.leave" && current {
			allowed[q.Kind] = []string{"conversationId", "userId", "membershipGeneration"}
		}
		fields, ok := allowed[q.Kind]
		if !ok || (q.Phase != "intent" && q.Phase != "result") || q.OperationKey == "" || len(q.OperationKey) > 200 {
			return fail(400, "INVALID_JOURNAL_RECORD", "Unrecognized safety fact")
		}
		owner := strings.SplitN(q.Kind, ".", 2)[0]
		if owner == "auth" {
			owner = "identity"
		}
		if q.Owner != owner || r.Header.Get("X-JobsIM-Caller") != owner {
			return fail(403, "JOURNAL_OWNER_MISMATCH", "Only the fact owner may append its safety record")
		}
		if len(q.Payload) != len(fields) {
			return fail(400, "INVALID_JOURNAL_PAYLOAD", "Only typed minimal safety fields are permitted")
		}
		for _, field := range fields {
			if _, ok = q.Payload[field]; !ok {
				return fail(400, "INVALID_JOURNAL_PAYLOAD", "Required safety field missing")
			}
			if e := validateJournalField(field, q.Payload[field]); e != nil {
				return e
			}
		}
		if q.Kind == "auth.account_state" && q.Payload["state"] != "active" && q.Payload["state"] != "banned" && q.Payload["state"] != "tombstoned" {
			return fail(400, "INVALID_JOURNAL_STATE", "Invalid account state")
		}
		if q.Kind == "governance.export_fence" {
			state, _ := q.Payload["state"].(string)
			if q.Phase == "result" && state != "cleared" || q.Phase == "intent" && state != "cancelled" && state != "invalidated" && state != "expired" && state != "failed" && state != "cancelling" {
				return fail(400, "INVALID_JOURNAL_STATE", "Invalid export fence phase/state")
			}
		}
		b, _ := json.Marshal(q)
		digest := tokenDigest(string(b))
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			return e
		}
		defer tx.Rollback(r.Context())
		var previous []byte
		if e = tx.QueryRow(r.Context(), `SELECT digest FROM journal.chain_head WHERE singleton FOR UPDATE`).Scan(&previous); e != nil {
			return e
		}
		var old []byte
		var seq int64
		e = tx.QueryRow(r.Context(), `SELECT seq,metadata_digest FROM journal.entries WHERE owner=$1 AND operation_key=$2 AND phase=$3`, q.Owner, q.OperationKey, q.Phase).Scan(&seq, &old)
		if e == nil {
			if !bytesEqual(old, digest) {
				return fail(409, "JOURNAL_CONFLICT", "Original journal operation differs")
			}
			respond(w, 200, map[string]string{"journalSeq": str(seq)})
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if q.Phase == "result" {
			var originalKind string
			var originalPayload []byte
			if e = tx.QueryRow(r.Context(), `SELECT kind,payload FROM journal.entries WHERE owner=$1 AND operation_key=$2 AND phase='intent'`, q.Owner, q.OperationKey).Scan(&originalKind, &originalPayload); e != nil || originalKind != q.Kind {
				return fail(409, "JOURNAL_INTENT_REQUIRED", "A result must continue its original typed intent")
			}
			var intent map[string]any
			if e = json.Unmarshal(originalPayload, &intent); e != nil {
				return e
			}
			for _, field := range fields {
				if q.Kind == "governance.export_fence" && field == "state" {
					continue
				}
				if intent[field] != q.Payload[field] {
					return fail(409, "JOURNAL_RESULT_MISMATCH", "Result differs from its original bound intent")
				}
			}
		}
		payload, _ := json.Marshal(q.Payload)
		chain := tokenDigest(string(append(append([]byte{}, previous...), digest...)))
		e = tx.QueryRow(r.Context(), `INSERT INTO journal.entries(owner,operation_key,phase,kind,payload,metadata_digest,previous_digest,entry_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING seq`, q.Owner, q.OperationKey, q.Phase, q.Kind, payload, digest, previous, chain).Scan(&seq)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE journal.chain_head SET digest=$1 WHERE singleton`, chain)
		if e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		respond(w, 200, map[string]string{"journalSeq": str(seq)})
		return nil
	}))
}

func validObjectID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	_, e := hex.DecodeString(strings.ReplaceAll(value, "-", ""))
	return e == nil
}
func validateJournalField(field string, raw any) error {
	value, ok := raw.(string)
	if !ok {
		return fail(400, "INVALID_JOURNAL_PAYLOAD", "Safety fields require typed strings")
	}
	valid := false
	switch {
	case strings.HasSuffix(field, "Id"):
		valid = validObjectID(value)
	case field == "username":
		key, e := normalizeUsername(value)
		valid = e == nil && key == value
	case field == "requestKey" || field == "replacementKey":
		valid = len(value) >= 8 && len(value) <= 128 && !strings.ContainsAny(value, "\r\n\x00")
	case field == "generation" || field == "membershipGeneration" || field == "securityVersion" || field == "version" || field == "step" || field == "restoreEpoch" || field == "buildGeneration" || field == "sourceVersion":
		n, e := strconv.ParseInt(value, 10, 64)
		valid = e == nil && n >= 0 && str(n) == value
		if field != "step" {
			valid = valid && n > 0
		}
	case field == "state":
		valid = value == "active" || value == "banned" || value == "tombstoned" || value == "cancelling" || value == "invalidated" || value == "expired" || value == "cleared" || value == "cancelled" || value == "failed"
	case field == "stateBefore":
		valid = value == "" || value == "active" || value == "banned"
	case field == "banUntil" || field == "expiresAt":
		_, e := time.Parse(time.RFC3339Nano, value)
		valid = e == nil || field == "banUntil" && value == ""
	}
	if !valid {
		return fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid minimal safety field: "+field)
	}
	return nil
}
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	v := byte(0)
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}
func VerifyIndependentJournal(ctx context.Context, business, journal string) error {
	if business == "" || journal == "" {
		return fmt.Errorf("business and independent journal PostgreSQL URLs are required")
	}
	ids := []string{}
	for _, dsn := range []string{business, journal} {
		conn, e := pgx.Connect(ctx, dsn)
		if e != nil {
			return fmt.Errorf("database identity probe unavailable")
		}
		var id string
		e = conn.QueryRow(ctx, `SELECT system_identifier::text FROM pg_control_system()`).Scan(&id)
		if e == nil {
			var fsync, fullPages string
			e = conn.QueryRow(ctx, `SELECT current_setting('fsync'),current_setting('full_page_writes')`).Scan(&fsync, &fullPages)
			if e == nil && (fsync != "on" || fullPages != "on") {
				e = fmt.Errorf("durability settings are not enabled")
			}
		}
		conn.Close(ctx)
		if e != nil {
			return fmt.Errorf("PostgreSQL system identifier probe requires an authorized operator")
		}
		ids = append(ids, id)
	}
	if ids[0] == ids[1] {
		return fmt.Errorf("journal must use a separate PostgreSQL cluster; a second database in the same cluster is insufficient")
	}
	return nil
}
