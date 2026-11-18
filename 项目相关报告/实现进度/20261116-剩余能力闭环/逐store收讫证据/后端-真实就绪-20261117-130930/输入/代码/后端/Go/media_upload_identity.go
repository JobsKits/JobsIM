package jobsim

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type mediaUploadAuth struct {
	Token         string `json:"token"`
	Method        string `json:"method"`
	Origin        string `json:"origin"`
	CSRF          string `json:"csrf"`
	Authorization string `json:"authorization"`
	Path          string `json:"path"`
	AuthFamily    string `json:"authFamily"`
	ContextLearn  string `json:"contextLearn"`
}

func (a *App) mediaUploadPrincipal(r *http.Request) (mediaUploadPrincipal, error) {
	q := mediaUploadAuth{bearer(r), r.Method, r.Header.Get("Origin"), r.Header.Get("X-CSRF-Token"), r.Header.Get("Authorization"), r.URL.Path, r.Header.Get("X-JobsIM-Auth-Family"), ""}
	var p mediaUploadPrincipal
	err := a.rpc(r.Context(), "identity", "/internal/media-upload/principal", q, &p)
	return p, err
}

func (a *App) mediaUploadIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/media-upload/principal", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "media" {
			return fail(403, "UPLOAD_OWNER_REQUIRED", "Only the current media owner may qualify this original principal")
		}
		var q mediaUploadAuth
		if err := decodeReceiptRequest(r, &q); err != nil {
			return err
		}
		raw, _ := json.Marshal(q)
		proof := r.Clone(r.Context())
		proof.Body = io.NopCloser(bytes.NewReader(raw))
		out := &personalExportBufferedResponse{header: make(http.Header)}
		if err := a.authorize(out, proof); err != nil {
			return err
		}
		var u User
		if err := json.Unmarshal(out.body.Bytes(), &u); err != nil {
			return err
		}
		token := q.Token
		if !strings.HasPrefix(q.Authorization, "Bearer ") {
			browser, err := a.browserSession(r.Context(), q.Token)
			if err != nil {
				return err
			}
			token = browser.Credentials.Token
		}
		var p mediaUploadPrincipal
		err := a.DB.QueryRow(r.Context(), `SELECT s.user_id::text,s.device_id::text,s.family_id::text,s.restore_epoch::text,s.security_version::text,s.source_generation::text FROM auth.sessions s JOIN auth.users u ON u.id=s.user_id JOIN auth.instance i ON i.singleton WHERE s.token_digest=$1 AND s.user_id=$2 AND s.device_id=$3 AND s.family_id IS NOT NULL AND s.revoked_at IS NULL AND NOT s.web_pending AND s.export_pending_operation_id IS NULL AND s.access_expires_at>clock_timestamp() AND s.expires_at>clock_timestamp() AND s.generation=u.credential_generation AND s.security_version=u.security_version AND s.restore_epoch=i.restore_epoch AND s.source_context_id='personal' AND s.source_generation=i.personal_source_generation AND u.state='active' AND u.recovery_state='normal' AND u.deletion_state='none' AND u.pending_operation_id IS NULL`, tokenDigest(token), u.ID, u.DeviceID).Scan(&p.UserID, &p.DeviceID, &p.FamilyID, &p.Epoch, &p.Security, &p.SourceGeneration)
		if err != nil {
			return fail(401, "UPLOAD_PRINCIPAL_CHANGED", "Current original upload family is unavailable")
		}
		respond(w, 200, p)
		return nil
	}))
}
