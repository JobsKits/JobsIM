package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func init() {
	migrations["directory"] += `
CREATE TABLE IF NOT EXISTS directory.source_preferences(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,
font_preset text NOT NULL DEFAULT 'standard' CHECK(font_preset IN('small','standard','large')),
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),row_version bigint NOT NULL DEFAULT 0 CHECK(row_version>=0),
explicit boolean NOT NULL DEFAULT false,updated_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id));
CREATE TABLE IF NOT EXISTS directory.source_preference_operations(
scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),request_id text NOT NULL CHECK(octet_length(request_id) BETWEEN 8 AND 128),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),result_version bigint NOT NULL CHECK(result_version>0),
font_preset text NOT NULL CHECK(font_preset IN('small','standard','large')),created_at timestamptz NOT NULL DEFAULT now(),
PRIMARY KEY(scope_id,source_context_id,user_id,revision_epoch,request_id));`
}

type sourcePreferenceView struct {
	ScopeID         string `json:"scopeId"`
	SourceContextID string `json:"sourceContextId"`
	UserID          string `json:"userId"`
	RevisionEpoch   string `json:"revisionEpoch"`
	Version         string `json:"version"`
	FontPreset      string `json:"fontPreset"`
	Explicit        bool   `json:"explicit"`
}

type sourcePreferencePatch struct {
	FontPreset      string `json:"fontPreset"`
	RevisionEpoch   string `json:"revisionEpoch"`
	ExpectedVersion string `json:"expectedVersion"`
	RequestID       string `json:"requestId"`
}

type sourcePreferenceResult struct {
	RevisionEpoch string `json:"revisionEpoch"`
	Version       string `json:"version"`
	FontPreset    string `json:"fontPreset"`
}

type sourcePreferenceOperation struct {
	OperationID string                 `json:"operationId"`
	Status      string                 `json:"status"`
	Result      sourcePreferenceResult `json:"result"`
	Current     bool                   `json:"current"`
}

type sourcePreferenceQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// The current source is server-derived; no body/query parameter selects another user or source.
func (a *App) sourcePreferencesRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/me/preferences", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		view, err := a.sourcePreferencesRead(r.Context(), u)
		if err != nil {
			return err
		}
		respond(w, 200, view)
		return nil
	})
	h("PATCH /api/me/preferences", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		var patch sourcePreferencePatch
		if err = decodeSourcePreferencePatch(r, &patch); err != nil {
			return err
		}
		operation, err := a.sourcePreferencesApply(r.Context(), u, patch)
		if err != nil {
			return err
		}
		respond(w, 200, operation)
		return nil
	})
	h("GET /api/me/preferences/operations/{requestId}", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		operation, err := a.sourcePreferencesOriginal(r.Context(), u, r.PathValue("requestId"), r.URL.Query().Get("revisionEpoch"))
		if err != nil {
			return err
		}
		respond(w, 200, operation)
		return nil
	})
	h("POST /api/me/preferences-results", func(w http.ResponseWriter, r *http.Request) error {
		u, err := a.current(r)
		if err != nil {
			return err
		}
		var patch sourcePreferencePatch
		if err = decodeSourcePreferencePatch(r, &patch); err != nil {
			return err
		}
		operation, err := a.sourcePreferencesProof(r.Context(), u, patch)
		if err != nil {
			return err
		}
		respond(w, 200, operation)
		return nil
	})
}

func decodeSourcePreferencePatch(r *http.Request, patch *sourcePreferencePatch) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(patch) != nil || decoder.Decode(new(any)) != io.EOF {
		return fail(400, "INVALID_JSON", "A single bounded preference request with supported fields required")
	}
	return nil
}

func sourcePreferenceCounter(raw string, allowZero bool) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 || !allowZero && value == 0 || str(value) != raw {
		return 0, fail(400, "INVALID_PREFERENCE_REVISION", "Exact decimal epoch and version strings required")
	}
	return value, nil
}

func (a *App) sourcePreferencePrincipal(u User) error {
	if !validObjectID(a.Config.DeploymentID) || !validObjectID(u.ID) {
		return fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated source instance unavailable")
	}
	_, err := sourcePreferenceCounter(u.RestoreEpoch, false)
	return err
}

// Old ordinary preferences retain their display choice but start the new epoch at version zero.
func (a *App) sourcePreferencesRead(ctx context.Context, u User) (sourcePreferenceView, error) {
	view := sourcePreferenceView{ScopeID: a.Config.DeploymentID, SourceContextID: "personal", UserID: u.ID}
	if err := a.sourcePreferencePrincipal(u); err != nil {
		return view, err
	}
	var scope string
	err := a.DB.QueryRow(ctx, `SELECT i.deployment_id,i.restore_epoch::text,COALESCE(p.font_preset,'standard'),CASE WHEN p.revision_epoch=i.restore_epoch THEN p.row_version ELSE 0 END::text,COALESCE(p.explicit,false)
FROM directory.instance i LEFT JOIN directory.source_preferences p ON p.scope_id=$1 AND p.source_context_id='personal' AND p.user_id=$2 WHERE i.singleton`, a.Config.DeploymentID, u.ID).Scan(&scope, &view.RevisionEpoch, &view.FontPreset, &view.Version, &view.Explicit)
	if err != nil {
		return view, err
	}
	if scope != view.ScopeID || view.RevisionEpoch != u.RestoreEpoch {
		return view, fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated and directory epochs do not agree")
	}
	return view, nil
}

func (a *App) sourcePreferenceFind(ctx context.Context, db sourcePreferenceQuery, user, request, epoch string) (sourcePreferenceOperation, []byte, bool, error) {
	operation := sourcePreferenceOperation{OperationID: request, Status: "applied"}
	var original []byte
	err := db.QueryRow(ctx, `SELECT request_digest,revision_epoch::text,result_version::text,font_preset FROM directory.source_preference_operations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND revision_epoch=$3 AND request_id=$4`, a.Config.DeploymentID, user, epoch, request).Scan(&original, &operation.Result.RevisionEpoch, &operation.Result.Version, &operation.Result.FontPreset)
	if err == pgx.ErrNoRows {
		return operation, nil, false, nil
	}
	return operation, original, err == nil, err
}

func (a *App) sourcePreferenceIsCurrent(ctx context.Context, db sourcePreferenceQuery, user, epoch string, operation sourcePreferenceOperation) (bool, error) {
	if operation.Result.RevisionEpoch != epoch {
		return false, nil
	}
	var current bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM directory.source_preferences WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND revision_epoch=$3 AND row_version=$4 AND font_preset=$5 AND explicit)`, a.Config.DeploymentID, user, epoch, operation.Result.Version, operation.Result.FontPreset).Scan(&current)
	return current, err
}

func (a *App) sourcePreferenceRequestDigest(u User, patch sourcePreferencePatch) ([]byte, error) {
	if err := a.sourcePreferencePrincipal(u); err != nil {
		return nil, err
	}
	if patch.FontPreset != "small" && patch.FontPreset != "standard" && patch.FontPreset != "large" {
		return nil, fail(400, "INVALID_FONT_PRESET", "Font preset must be small, standard, or large")
	}
	if len(patch.RequestID) < 8 || len(patch.RequestID) > 128 {
		return nil, fail(400, "REQUEST_ID_REQUIRED", "Stable requestId of 8–128 bytes required")
	}
	if _, err := sourcePreferenceCounter(patch.RevisionEpoch, false); err != nil {
		return nil, err
	}
	if _, err := sourcePreferenceCounter(patch.ExpectedVersion, true); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	return keyedDigest(a.Config.Key, "source-preference-v1:"+a.Config.DeploymentID+":personal:"+u.ID, canonical), nil
}

// Proof is read-only: a missing original request cannot create a preference.
func (a *App) sourcePreferencesProof(ctx context.Context, u User, patch sourcePreferencePatch) (sourcePreferenceOperation, error) {
	var operation sourcePreferenceOperation
	digest, err := a.sourcePreferenceRequestDigest(u, patch)
	if err != nil {
		return operation, err
	}
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	var epoch string
	if err = tx.QueryRow(ctx, `SELECT restore_epoch::text FROM directory.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); err != nil {
		return operation, err
	}
	if epoch != u.RestoreEpoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated and directory epochs do not agree")
	}
	operation, original, found, err := a.sourcePreferenceFind(ctx, tx, u.ID, patch.RequestID, patch.RevisionEpoch)
	if err != nil {
		return operation, err
	}
	if !found {
		return operation, fail(404, "PREFERENCE_OPERATION_NOT_FOUND", "No original preference operation exists")
	}
	if !hmac.Equal(original, digest) {
		return operation, fail(409, "IDEMPOTENCY_CONFLICT", "Original preference request differs")
	}
	operation.Current, err = a.sourcePreferenceIsCurrent(ctx, tx, u.ID, epoch, operation)
	return operation, err
}

// A single read snapshot binds original metadata and its current flag to the authenticated epoch.
func (a *App) sourcePreferencesOriginal(ctx context.Context, u User, request, epoch string) (sourcePreferenceOperation, error) {
	operation := sourcePreferenceOperation{Status: "applied"}
	if err := a.sourcePreferencePrincipal(u); err != nil {
		return operation, err
	}
	if len(request) < 8 || len(request) > 128 {
		return operation, fail(400, "REQUEST_ID_REQUIRED", "Stable requestId of 8–128 bytes required")
	}
	if _, err := sourcePreferenceCounter(epoch, false); err != nil {
		return operation, err
	}
	var scope, currentEpoch string
	err := a.DB.QueryRow(ctx, `SELECT i.deployment_id,i.restore_epoch::text,COALESCE(o.request_id,''),COALESCE(o.revision_epoch,0)::text,COALESCE(o.result_version,0)::text,COALESCE(o.font_preset,''),COALESCE(o.revision_epoch=i.restore_epoch AND p.revision_epoch=i.restore_epoch AND p.row_version=o.result_version AND p.font_preset=o.font_preset AND p.explicit,false)
FROM directory.instance i LEFT JOIN directory.source_preference_operations o ON o.scope_id=$1 AND o.source_context_id='personal' AND o.user_id=$2 AND o.revision_epoch=$3 AND o.request_id=$4
LEFT JOIN directory.source_preferences p ON p.scope_id=$1 AND p.source_context_id='personal' AND p.user_id=$2 WHERE i.singleton`, a.Config.DeploymentID, u.ID, epoch, request).Scan(&scope, &currentEpoch, &operation.OperationID, &operation.Result.RevisionEpoch, &operation.Result.Version, &operation.Result.FontPreset, &operation.Current)
	if err != nil {
		return operation, err
	}
	if scope != a.Config.DeploymentID || currentEpoch != u.RestoreEpoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated and directory epochs do not agree")
	}
	if operation.OperationID == "" {
		return operation, fail(404, "PREFERENCE_OPERATION_NOT_FOUND", "No original preference operation exists")
	}
	return operation, nil
}

// The preference row serializes CAS and nonce lookup; the operation fact commits with the value.
func (a *App) sourcePreferencesApply(ctx context.Context, u User, patch sourcePreferencePatch) (sourcePreferenceOperation, error) {
	var operation sourcePreferenceOperation
	digest, err := a.sourcePreferenceRequestDigest(u, patch)
	if err != nil {
		return operation, err
	}
	expected, _ := sourcePreferenceCounter(patch.ExpectedVersion, true)
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return operation, err
	}
	defer tx.Rollback(ctx)
	var scope, epoch string
	if err = tx.QueryRow(ctx, `SELECT deployment_id,restore_epoch::text FROM directory.instance WHERE singleton FOR SHARE`).Scan(&scope, &epoch); err != nil {
		return operation, err
	}
	if scope != a.Config.DeploymentID || epoch != u.RestoreEpoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Authenticated and directory epochs do not agree")
	}
	lookup := func() (bool, error) {
		var original []byte
		var found bool
		operation, original, found, err = a.sourcePreferenceFind(ctx, tx, u.ID, patch.RequestID, patch.RevisionEpoch)
		if err != nil || !found {
			return found, err
		}
		if !hmac.Equal(original, digest) {
			return true, fail(409, "IDEMPOTENCY_CONFLICT", "Original preference request differs")
		}
		operation.Current, err = a.sourcePreferenceIsCurrent(ctx, tx, u.ID, epoch, operation)
		return true, err
	}
	if found, lookupError := lookup(); found || lookupError != nil {
		if lookupError != nil {
			return operation, lookupError
		}
		return operation, tx.Commit(ctx)
	}
	if patch.RevisionEpoch != epoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Uncommitted preference request belongs to an old epoch")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO directory.source_preferences(scope_id,source_context_id,user_id,revision_epoch) VALUES($1,'personal',$2,$3) ON CONFLICT DO NOTHING`, scope, u.ID, epoch); err != nil {
		return operation, err
	}
	var rowEpoch string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT revision_epoch::text,row_version FROM directory.source_preferences WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 FOR UPDATE`, scope, u.ID).Scan(&rowEpoch, &version); err != nil {
		return operation, err
	}
	// A concurrent first request may have committed while INSERT/FOR UPDATE waited.
	if found, lookupError := lookup(); found || lookupError != nil {
		if lookupError != nil {
			return operation, lookupError
		}
		return operation, tx.Commit(ctx)
	}
	if rowEpoch != epoch {
		version = 0
	}
	if version != expected || version == math.MaxInt64 {
		return operation, fail(409, "PREFERENCE_VERSION_CONFLICT", "Reload current preference epoch and version")
	}
	operation = sourcePreferenceOperation{OperationID: patch.RequestID, Status: "applied", Current: true, Result: sourcePreferenceResult{RevisionEpoch: epoch, Version: str(version + 1), FontPreset: patch.FontPreset}}
	if _, err = tx.Exec(ctx, `UPDATE directory.source_preferences SET font_preset=$3,revision_epoch=$4,row_version=$5,explicit=true,updated_at=now() WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2`, scope, u.ID, patch.FontPreset, epoch, version+1); err != nil {
		return operation, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO directory.source_preference_operations(scope_id,source_context_id,user_id,revision_epoch,request_id,request_digest,result_version,font_preset) VALUES($1,'personal',$2,$3,$4,$5,$6,$7)`, scope, u.ID, epoch, patch.RequestID, digest, version+1, patch.FontPreset); err != nil {
		return operation, err
	}
	return operation, tx.Commit(ctx)
}
