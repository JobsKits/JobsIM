package jobsim

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strconv"
)

type AvatarProfile struct {
	DisplayName        string `json:"displayName"`
	ProfileVersion     string `json:"profileVersion"`
	AvatarURL          string `json:"avatarURL"`
	AvatarThumbnailURL string `json:"avatarThumbnailURL"`
}

func validAssetID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (a *App) avatarProfileFields(ctx context.Context, user string) (AvatarProfile, error) {
	v := AvatarProfile{ProfileVersion: "1"}
	var root, thumbnail *string
	var version int64
	e := a.DB.QueryRow(ctx, `SELECT display_name,row_version,avatar_object_id::text,avatar_thumbnail_id::text FROM directory.profiles WHERE user_id=$1`, user).Scan(&v.DisplayName, &version, &root, &thumbnail)
	if e == pgx.ErrNoRows {
		return v, nil
	}
	if e != nil {
		return v, e
	}
	v.ProfileVersion = str(version)
	if root != nil {
		v.AvatarURL = "/api/avatars/" + *root
	}
	if thumbnail != nil {
		v.AvatarThumbnailURL = "/api/avatars/" + *thumbnail
	}
	return v, nil
}

func avatarResultUser(u User, p AvatarProfile) map[string]any {
	name := p.DisplayName
	if name == "" {
		name = u.DisplayName
	}
	return map[string]any{"id": u.ID, "username": u.Username, "displayName": name, "profileVersion": p.ProfileVersion, "avatarURL": p.AvatarURL, "avatarThumbnailURL": p.AvatarThumbnailURL}
}

func (a *App) avatarDirectoryRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /api/me/avatar", a.publishAvatar)
	h("GET /api/me/avatar/operations/{id}", func(w http.ResponseWriter, r *http.Request) error {
		u, e := a.current(r)
		if e != nil {
			return e
		}
		return a.respondAvatarOperation(w, r, u, r.PathValue("id"))
	})
	h("POST /internal/avatar/profile", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID string `json:"userId"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		v, e := a.avatarProfileFields(r.Context(), q.UserID)
		if e != nil {
			return e
		}
		respond(w, 200, v)
		return nil
	}))
	h("POST /internal/avatar/prepared", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			UserID       string `json:"userId"`
			OperationID  string `json:"operationId"`
			Generation   string `json:"generation"`
			RestoreEpoch string `json:"restoreEpoch"`
			SourceDigest []byte `json:"sourceDigest"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		var valid bool
		e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.avatar_operations o JOIN directory.profiles p ON p.user_id=o.user_id WHERE o.user_id=$1 AND o.request_id=$2 AND o.state='prepared' AND p.pending_avatar_operation_id=o.request_id AND p.avatar_generation=$3::bigint AND o.generation=p.avatar_generation AND o.restore_epoch=$4::bigint AND o.source_digest=$5 AND p.revision_epoch=o.restore_epoch)`, q.UserID, q.OperationID, q.Generation, q.RestoreEpoch, q.SourceDigest).Scan(&valid)
		if e != nil {
			return e
		}
		if !valid {
			return fail(409, "AVATAR_SUPERSEDED", "Avatar operation is no longer prepared")
		}
		respond(w, 200, map[string]bool{"prepared": true})
		return nil
	}))
	h("POST /internal/avatar/access", internal(a.avatarAccess))
}

func (a *App) publishAvatar(w http.ResponseWriter, r *http.Request) error {
	u, e := a.current(r)
	if e != nil {
		return e
	}
	r.Body = http.MaxBytesReader(w, r.Body, (2<<20)+(64<<10))
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return fail(413, "AVATAR_UPLOAD_LIMIT", "Avatar upload limit is 2 MiB")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	request, expected := r.FormValue("requestId"), r.FormValue("expectedVersion")
	if len(request) < 8 || len(request) > 128 || expected == "" {
		return fail(400, "AVATAR_OPERATION_REQUIRED", "Stable requestId and expectedVersion required")
	}
	f, _, e := r.FormFile("file")
	if e != nil {
		return fail(400, "FILE_REQUIRED", "Multipart file field required")
	}
	defer f.Close()
	content, e := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	if e != nil {
		return e
	}
	if len(content) > 2<<20 {
		return fail(413, "AVATAR_UPLOAD_LIMIT", "Avatar upload limit is 2 MiB")
	}
	source := sha256.Sum256(content)
	raw, _ := json.Marshal(map[string]string{"sourceDigest": stringBase64(source[:]), "expectedVersion": expected})
	digest := keyedDigest(a.Config.Key, "avatar-publish:"+u.ID, raw)
	epoch, e := strconv.ParseInt(u.RestoreEpoch, 10, 64)
	if e != nil {
		return e
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `INSERT INTO directory.profiles(user_id,display_name) VALUES($1,$2) ON CONFLICT DO NOTHING`, u.ID, u.DisplayName)
	if e != nil {
		return e
	}
	var version, generation int64
	var pending *string
	if e = tx.QueryRow(r.Context(), `SELECT row_version,avatar_generation,pending_avatar_operation_id FROM directory.profiles WHERE user_id=$1 FOR UPDATE`, u.ID).Scan(&version, &generation, &pending); e != nil {
		return e
	}
	var original []byte
	var state string
	var oldEpoch int64
	e = tx.QueryRow(r.Context(), `SELECT request_digest,state,generation,restore_epoch FROM directory.avatar_operations WHERE user_id=$1 AND request_id=$2`, u.ID, request).Scan(&original, &state, &generation, &oldEpoch)
	if e == nil {
		if !bytes.Equal(original, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original avatar request differs")
		}
		if state != "prepared" {
			tx.Rollback(r.Context())
			return a.respondAvatarOperation(w, r, u, request)
		}
		if oldEpoch != epoch {
			return fail(409, "RESTORE_EPOCH_CHANGED", "Avatar operation belongs to an earlier restore")
		}
	} else if e == pgx.ErrNoRows {
		if str(version) != expected {
			return fail(409, "VERSION_CONFLICT", "Reload the current profile")
		}
		generation++
		if pending != nil {
			result, _ := json.Marshal(map[string]any{"status": "superseded", "operationId": *pending})
			if _, e = tx.Exec(r.Context(), `UPDATE directory.avatar_operations SET state='superseded',result=$3 WHERE user_id=$1 AND request_id=$2 AND state='prepared'`, u.ID, *pending, result); e != nil {
				return e
			}
		}
		_, e = tx.Exec(r.Context(), `INSERT INTO directory.avatar_operations(user_id,request_id,request_digest,source_digest,restore_epoch,generation,state) VALUES($1,$2,$3,$4,$5,$6,'prepared')`, u.ID, request, digest, source[:], epoch, generation)
		if e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE directory.profiles SET pending_avatar_operation_id=$2,avatar_generation=$3,revision_epoch=$4 WHERE user_id=$1`, u.ID, request, generation, epoch)
		if e != nil {
			return e
		}
	} else {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	var outputs struct {
		RootID      string `json:"rootId"`
		ThumbnailID string `json:"thumbnailId"`
	}
	process := avatarProcessRequest{UserID: u.ID, OperationID: request, Generation: str(generation), RestoreEpoch: u.RestoreEpoch, SourceDigest: source[:], Content: content}
	if e = a.rpc(r.Context(), "media", "/internal/avatar/process", process, &outputs); e != nil {
		var rejected *apiError
		if errors.As(e,&rejected)&&rejected.Status==409&&rejected.Code=="AVATAR_SUPERSEDED"{
			if _,ce:=a.current(r);ce!=nil{return ce}
			return a.respondAvatarOperation(w,r,u,request)
		}
		if errors.As(e, &rejected) && (rejected.Status == 400 || rejected.Status == 413) {
			// Deterministic decode failures end only their own preparation; old live avatars remain intact.
			cancelTx, ce := a.DB.Begin(r.Context())
			if ce != nil {
				return ce
			}
			defer cancelTx.Rollback(r.Context())
			if _, ce = cancelTx.Exec(r.Context(), `SELECT user_id FROM directory.profiles WHERE user_id=$1 FOR UPDATE`, u.ID); ce != nil {
				return ce
			}
			cancelled, _ := json.Marshal(map[string]any{"status": "cancelled", "operationId": request, "errorCode": rejected.Code})
			if _, ce = cancelTx.Exec(r.Context(), `UPDATE directory.avatar_operations SET state='cancelled',result=$3 WHERE user_id=$1 AND request_id=$2 AND state='prepared'`, u.ID, request, cancelled); ce != nil {
				return ce
			}
			if _, ce = cancelTx.Exec(r.Context(), `UPDATE directory.profiles SET pending_avatar_operation_id=NULL WHERE user_id=$1 AND pending_avatar_operation_id=$2`, u.ID, request); ce != nil {
				return ce
			}
			if ce = cancelTx.Commit(r.Context()); ce != nil {
				return ce
			}
		}
		return e
	}
	// Cross-owner validation completes before the profile lock; publication is then one owner CAS.
	if _, e = a.current(r); e != nil {
		return e
	}
	tx, e = a.DB.Begin(r.Context())
	if e != nil {
		return e
	}
	defer tx.Rollback(r.Context())
	if e = tx.QueryRow(r.Context(), `SELECT row_version,pending_avatar_operation_id FROM directory.profiles WHERE user_id=$1 FOR UPDATE`, u.ID).Scan(&version, &pending); e != nil {
		return e
	}
	var currentGeneration, currentEpoch int64
	if e = tx.QueryRow(r.Context(), `SELECT avatar_generation,revision_epoch FROM directory.profiles WHERE user_id=$1`, u.ID).Scan(&currentGeneration, &currentEpoch); e != nil {
		return e
	}
	if pending == nil || *pending != request || currentGeneration != generation || currentEpoch != epoch {
		tx.Rollback(r.Context())
		return a.respondAvatarOperation(w, r, u, request)
	}
	version++
	_, e = tx.Exec(r.Context(), `UPDATE directory.profiles SET avatar_object_id=$2,avatar_thumbnail_id=$3,pending_avatar_operation_id=NULL,row_version=$4,updated_at=now() WHERE user_id=$1`, u.ID, outputs.RootID, outputs.ThumbnailID, version)
	if e != nil {
		return e
	}
	result, _ := json.Marshal(map[string]any{"status": "applied", "operationId": request, "avatarId": outputs.RootID, "thumbnailId": outputs.ThumbnailID, "profileVersion": str(version), "restoreEpoch": u.RestoreEpoch, "generation": str(generation)})
	_, e = tx.Exec(r.Context(), `UPDATE directory.avatar_operations SET state='completed',result=$3 WHERE user_id=$1 AND request_id=$2 AND state='prepared'`, u.ID, request, result)
	if e != nil {
		return e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return e
	}
	return a.respondAvatarOperation(w, r, u, request)
}

func (a *App) respondAvatarOperation(w http.ResponseWriter, r *http.Request, u User, request string) error {
	var state string
	var raw []byte
	if e := a.DB.QueryRow(r.Context(), `SELECT state,result FROM directory.avatar_operations WHERE user_id=$1 AND request_id=$2`, u.ID, request).Scan(&state, &raw); e != nil {
		return fail(404, "AVATAR_OPERATION_NOT_FOUND", "Avatar operation unavailable")
	}
	var original map[string]any
	if e := json.Unmarshal(raw, &original); e != nil {
		return e
	}
	profile, e := a.avatarProfileFields(r.Context(), u.ID)
	if e != nil {
		return e
	}
	root, _ := original["avatarId"].(string)
	current := root != "" && profile.AvatarURL == "/api/avatars/"+root
	status := state
	if state == "completed" {
		status = "applied"
	}
	respond(w, 200, map[string]any{"user": avatarResultUser(u, profile), "operationId": request, "status": status, "current": current, "result": original})
	return nil
}

func (a *App) avatarAccess(w http.ResponseWriter, r *http.Request) error {
	var q struct {
		UserID   string `json:"userId"`
		ObjectID string `json:"objectId"`
	}
	if e := decode(r, &q); e != nil {
		return e
	}
	if !validAssetID(q.ObjectID) || !validAssetID(q.UserID) {
		return fail(404, "AVATAR_UNAVAILABLE", "Avatar unavailable")
	}
	var owner string
	if e := a.DB.QueryRow(r.Context(), `SELECT user_id::text FROM directory.profiles WHERE avatar_object_id=$1 OR avatar_thumbnail_id=$1`, q.ObjectID).Scan(&owner); e != nil {
		return fail(404, "AVATAR_UNAVAILABLE", "Avatar unavailable")
	}
	var users any
	if e := a.rpc(r.Context(), "identity", "/internal/users", map[string]any{"ids": []string{owner}}, &users); e != nil {
		return fail(404, "AVATAR_UNAVAILABLE", "Avatar owner unavailable")
	}
	if owner != q.UserID {
		var blocked, friends bool
		e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)),EXISTS(SELECT 1 FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 AND state='friend')`, owner, q.UserID).Scan(&blocked, &friends)
		if e != nil {
			return e
		}
		if blocked {
			return fail(403, "AVATAR_FORBIDDEN", "Profile visibility required")
		}
		if !friends {
			var visible struct {
				Visible bool `json:"visible"`
			}
			if e = a.rpc(r.Context(), "conversation", "/internal/profile-visible", map[string]string{"first": owner, "second": q.UserID}, &visible); e != nil {
				return e
			}
			if !visible.Visible {
				return fail(403, "AVATAR_FORBIDDEN", "Profile visibility required")
			}
		}
	}
	respond(w, 200, map[string]string{"ownerId": owner})
	return nil
}

func (a *App) avatarConversationRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/profile-visible", internal(func(w http.ResponseWriter, r *http.Request) error {
		var q struct {
			First  string `json:"first"`
			Second string `json:"second"`
		}
		if e := decode(r, &q); e != nil {
			return e
		}
		var visible bool
		e := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM conversation.members a JOIN conversation.members b ON b.conversation_id=a.conversation_id JOIN conversation.conversations c ON c.id=a.conversation_id WHERE a.user_id=$1 AND b.user_id=$2 AND a.state='active' AND b.state='active' AND c.state='active')`, q.First, q.Second).Scan(&visible)
		if e != nil {
			return e
		}
		respond(w, 200, map[string]bool{"visible": visible})
		return nil
	}))
}
