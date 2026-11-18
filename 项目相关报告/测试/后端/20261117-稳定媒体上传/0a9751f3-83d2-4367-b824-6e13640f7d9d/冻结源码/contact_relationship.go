package jobsim

import (
	"net/http"
	"strings"
)

// ContactRelationship describes only the authenticated owner's stored policy;
// it is not a grant to communicate or read the peer's profile.
type ContactRelationship struct {
	PeerID          string `json:"peerId"`
	BlockedByMe     bool   `json:"blockedByMe"`
	Friend          bool   `json:"friend"`
	InboundGate     string `json:"inboundGate"`
	Generation      string `json:"generation"`
	InboundReopened bool   `json:"inboundReopened"`
}

func (a *App) contactRelationship(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	peer := strings.ToLower(r.PathValue("userId"))
	if !validObjectID(peer) || peer == u.ID {
		return fail(400, "INVALID_PEER", "A different stable peer ID required")
	}
	v := ContactRelationship{PeerID: peer}
	var generation int64
	// One statement gives a coherent read without creating default contact rows.
	// Every policy lookup is constrained to owner_id=$1; peer policy is private.
	e = a.DB.QueryRow(r.Context(), `SELECT
 EXISTS(SELECT 1 FROM directory.blocks WHERE owner_id=$1 AND peer_id=$2),
 COALESCE((SELECT state='friend' FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2),false),
 COALESCE((SELECT request_gate FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2),'open'),
 COALESCE((SELECT request_generation FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2),0),
 EXISTS(SELECT 1 FROM directory.contacts c JOIN directory.request_gate_operations o ON o.owner_id=c.owner_id AND o.peer_id=c.peer_id WHERE c.owner_id=$1 AND c.peer_id=$2 AND c.request_gate='open' AND o.result->>'generation'=c.request_generation::text)`, u.ID, peer).Scan(&v.BlockedByMe, &v.Friend, &v.InboundGate, &generation, &v.InboundReopened)
	if e != nil {
		return e
	}
	v.Generation = str(generation)
	respond(w, http.StatusOK, v)
	return nil
}
