package jobsim

import (
	"bytes"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"net/http"
)

func init() {
	migrations["directory"] += `CREATE TABLE IF NOT EXISTS directory.request_gate_operations(owner_id uuid NOT NULL,request_id text NOT NULL,peer_id uuid NOT NULL,request_digest bytea NOT NULL,result jsonb NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(owner_id,request_id));`
}

// This is an inbound permission owned by the authenticated recipient. It never
// changes the peer's inbound policy or resurrects a former accepted request.
func (a *App) reopenRequestGate(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	peer := r.PathValue("userId")
	if !validObjectID(peer) || peer == u.ID {
		return fail(400, "INVALID_PEER", "A different stable peer ID required")
	}
	var q struct {
		RequestID string `json:"requestId"`
	}
	if e = decode(r, &q); e != nil {
		return e
	}
	if len(q.RequestID) < 8 || len(q.RequestID) > 128 {
		return fail(400, "REQUEST_ID_REQUIRED", "A stable new requestId of 8–128 bytes required")
	}
	digest := keyedDigest(a.Config.Key, "inbound-request-reopen:"+u.ID, []byte(peer))
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = relationLock(r.Context(), tx, u.ID, peer); e != nil {
		return e
	}
	var original, result []byte
	var originalPeer string
	e = tx.QueryRow(r.Context(), `SELECT peer_id::text,request_digest,result FROM directory.request_gate_operations WHERE owner_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&originalPeer, &original, &result)
	if e == nil {
		if originalPeer != peer || !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original inbound gate operation differs")
		}
		if e = tx.Commit(r.Context()); e != nil {
			return e
		}
		return a.respondRequestGate(w, r, u.ID, peer, result)
	}
	if e != pgx.ErrNoRows {
		return e
	}
	var blocked bool
	if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1))`, u.ID, peer).Scan(&blocked); e != nil {
		return e
	}
	if blocked {
		return fail(403, "BLOCKED_RELATIONSHIP", "Remove the applicable block before explicitly reopening inbound requests")
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO directory.contacts(owner_id,peer_id,state) VALUES($1,$2,'removed') ON CONFLICT DO NOTHING`, u.ID, peer)
	if e != nil {
		return e
	}
	var generation int64
	if e = tx.QueryRow(r.Context(), `UPDATE directory.contacts SET request_gate='open',request_generation=request_generation+1,updated_at=now() WHERE owner_id=$1 AND peer_id=$2 RETURNING request_generation`, u.ID, peer).Scan(&generation); e != nil {
		return e
	}
	_, e = tx.Exec(r.Context(), `UPDATE directory.contact_requests SET state='rejected' WHERE requester_id=$2 AND recipient_id=$1 AND state='pending'`, u.ID, peer)
	if e != nil {
		return e
	}
	result, _ = json.Marshal(map[string]any{"status": "opened", "peerId": peer, "generation": str(generation), "requestId": q.RequestID, "direction": "inbound"})
	_, e = tx.Exec(r.Context(), `INSERT INTO directory.request_gate_operations(owner_id,request_id,peer_id,request_digest,result) VALUES($1,$2,$3,$4,$5)`, u.ID, q.RequestID, peer, digest, result)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	return a.respondRequestGate(w, r, u.ID, peer, result)
}

func (a *App) respondRequestGate(w http.ResponseWriter, r *http.Request, owner, peer string, result []byte) error {
	var body map[string]any
	if e := json.Unmarshal(result, &body); e != nil {
		return e
	}
	var current bool
	e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 AND request_gate='open' AND request_generation=$3::bigint) AND NOT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1))`, owner, peer, body["generation"]).Scan(&current)
	if e != nil {
		return e
	}
	body["current"] = current
	respond(w, 200, body)
	return nil
}
