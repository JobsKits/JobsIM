package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

var initialFeatures = map[string]bool{"messages": true, "groups": true, "media": true, "avatars": true, "reports": true, "search": true, "reactions": true, "favorites": true, "pins": true, "text_forward": false, "registration": true, "e2ee": false, "after_receipt": false, "rtc": false, "email_otp": false, "remote_push": false, "strong_protection": false, "safe_restore": false, "account_metadata_export": true, "chat_export": true, "independent_journal": true, "admin_writes": true}

func (a *App) features(ctx context.Context) (map[string]bool, error) {
	items := map[string]bool{}
	items["send_admission"] = true
	items["role_admission"] = true
	items["forwarding"] = true
	items["auth_refresh"] = true
	items["auth_retirement_chain"] = true
	items["mentions"] = true
	items["announcements"] = true
	items["authoritative_sync"] = a.Config.NATSURL != ""
	items["sync_jetstream"] = a.Config.NATSURL != ""
	items["sync_self_state"] = a.Config.NATSURL != ""
	items["cloud_drafts"] = a.Config.NATSURL != ""
	items["durable_receipt_protocol"] = a.Config.NATSURL != ""
	items["durable_object_receipt_protocol"] = a.Config.NATSURL != ""
	for k, v := range initialFeatures {
		items[k] = v
	}
	rows, e := a.DB.Query(ctx, `SELECT name,enabled FROM control.capabilities`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var enabled bool
		if e = rows.Scan(&name, &enabled); e != nil {
			return nil, e
		}
		mutable := map[string]bool{"groups": true, "media": true, "search": true, "reactions": true, "favorites": true, "pins": true, "registration": true}
		if mutable[name] {
			items[name] = enabled
		}
	}
	return items, rows.Err()
}
func (a *App) requireFeature(ctx context.Context, name string) error {
	var result struct {
		Enabled bool `json:"enabled"`
	}
	if e := a.rpc(ctx, "control", "/internal/capability", map[string]string{"name": name}, &result); e != nil {
		return e
	}
	if !result.Enabled {
		return fail(403, "FEATURE_DISABLED", "Feature disabled")
	}
	return nil
}
func (a *App) controlRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) error {
		f, e := a.features(r.Context())
		if e != nil {
			return e
		}
		epoch, e := a.epoch(r.Context())
		if e != nil {
			return e
		}
		respond(w, 200, map[string]any{"restoreEpoch": str(epoch), "deploymentId": a.Config.DeploymentID, "name": a.Config.Name, "features": f, "protocolVersion": "1"})
		return nil
	})
	h("GET /api/capabilities", func(w http.ResponseWriter, r *http.Request) error {
		if _, e := a.current(r); e != nil {
			return e
		}
		f, e := a.features(r.Context())
		if e != nil {
			return e
		}
		versions := map[string]string{}
		rows, e := a.DB.Query(r.Context(), `SELECT name,version,enabled FROM control.capabilities`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var name string
			var version int64
			var enabled bool
			if e = rows.Scan(&name, &version, &enabled); e != nil {
				rows.Close()
				return e
			}
			versions[name] = str(version)
			if name == "groups" || name == "media" || name == "search" || name == "reactions" || name == "favorites" || name == "pins" || name == "registration" {
				f[name] = enabled
			}
		}
		rows.Close()
		respond(w, 200, map[string]any{"features": f, "versions": versions})
		return nil
	})
	h("POST /internal/capability", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			Name string `json:"name"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		f, e := a.features(r.Context())
		if e != nil {
			return e
		}
		respond(w, 200, map[string]bool{"enabled": f[q.Name]})
		return nil
	}))
	h("PUT /api/capabilities/{name}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		allowed := map[string]bool{"groups": true, "media": true, "search": true, "reactions": true, "favorites": true, "pins": true, "registration": true}
		name := r.PathValue("name")
		if !allowed[name] {
			return fail(400, "IMMUTABLE_CAPABILITY", "Core safety and unimplemented capabilities cannot be changed")
		}
		var q struct {
			Enabled         *bool  `json:"enabled"`
			ExpectedVersion string `json:"expectedVersion"`
			RequestID       string `json:"requestId"`
			StepUpID        string `json:"stepUpId"`
		}
		raw, e := readAdminBody(r, &q)
		if e != nil {
			return e
		}
		if q.Enabled == nil || q.ExpectedVersion == "" || len(q.RequestID) < 8 || len(q.RequestID) > 128 {
			return fail(400, "INVALID_CAPABILITY_OPERATION", "Enabled, expectedVersion and stable requestId required")
		}
		canonical, e := canonicalAdminPayload(raw)
		if e != nil {
			return e
		}
		digest := keyedDigest(a.Config.Key, "capability:"+u.ID+":"+name, canonical)
		var completedDigest, completedResult []byte
		var completedActor string
		lookup := a.DB.QueryRow(r.Context(), `SELECT actor_id::text,request_digest,result FROM control.operation_results WHERE request_id=$1 AND state='completed'`, q.RequestID).Scan(&completedActor, &completedDigest, &completedResult)
		if lookup == nil {
			if completedActor != u.ID || !bytes.Equal(completedDigest, digest) {
				return fail(409, "IDEMPOTENCY_CONFLICT", "Original operation differs")
			}
			var body any
			json.Unmarshal(completedResult, &body)
			respond(w, 200, body)
			return nil
		}
		if e = a.previewAdminStepUp(r, u, q.StepUpID, "capability.update", name, q.RequestID, raw); e != nil {
			return e
		}
		_, e = a.DB.Exec(r.Context(), `INSERT INTO control.operation_results(request_id,actor_id,request_digest,state) VALUES($1,$2,$3,'prepared') ON CONFLICT DO NOTHING`, q.RequestID, u.ID, digest)
		if e != nil {
			return e
		}
		var original, result []byte
		var actor, state string
		if e = a.DB.QueryRow(r.Context(), `SELECT actor_id::text,request_digest,state,result FROM control.operation_results WHERE request_id=$1`, q.RequestID).Scan(&actor, &original, &state, &result); e != nil {
			return e
		}
		if actor != u.ID || !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original capability operation differs")
		}
		if state == "completed" {
			var body any
			json.Unmarshal(result, &body)
			respond(w, 200, body)
			return nil
		}
		if e = a.adminStepUp(r, u, q.StepUpID, "capability.update", name, q.RequestID, raw); e != nil {
			return e
		}
		tx, e := a.DB.Begin(r.Context())
		if e != nil {
			return e
		}
		defer tx.Rollback(r.Context())
		if e = tx.QueryRow(r.Context(), `SELECT state,result FROM control.operation_results WHERE request_id=$1 FOR UPDATE`, q.RequestID).Scan(&state, &result); e != nil {
			return e
		}
		if state == "completed" {
			var body any
			json.Unmarshal(result, &body)
			respond(w, 200, body)
			return nil
		}
		var version int64
		if e = tx.QueryRow(r.Context(), `SELECT version FROM control.capabilities WHERE name=$1 FOR UPDATE`, name).Scan(&version); e != nil {
			return e
		}
		if str(version) != q.ExpectedVersion {
			return fail(409, "VERSION_CONFLICT", "Capability changed; refresh and authorize a new operation")
		}
		version++
		_, e = tx.Exec(r.Context(), `UPDATE control.capabilities SET enabled=$2,version=$3,updated_at=now() WHERE name=$1`, name, *q.Enabled, version)
		if e != nil {
			return e
		}
		body := map[string]any{"name": name, "enabled": *q.Enabled, "version": str(version), "requestId": q.RequestID}
		result, _ = json.Marshal(body)
		_, e = tx.Exec(r.Context(), `UPDATE control.operation_results SET state='completed',result=$2 WHERE request_id=$1`, q.RequestID, result)
		if e != nil {
			return e
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		respond(w, 200, body)
		return nil
	})
	h("GET /api/admin/status", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		if !u.Admin {
			return fail(403, "ADMIN_REQUIRED", "Administrator required")
		}
		epoch, e := a.epoch(r.Context())
		if e != nil {
			return e
		}
		respond(w, 200, map[string]any{"deploymentId": a.Config.DeploymentID, "restoreEpoch": str(epoch), "protectionProfile": "basic_self_hosted", "readiness": "development_slice", "postgres": true, "independentJournal": true, "administratorMFA": true, "safeRestore": false, "unverified": []string{"full_recovery_asset_coverage", "per_domain_least_privilege", "cross_host_service_identity", "full_snapshot_sync", "complete_source_lifecycle", "strong_protection"}})
		return nil
	})
}
