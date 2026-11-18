package jobsim

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func init() {
	migrations["identity"] += `CREATE TABLE IF NOT EXISTS auth.username_operations(user_id uuid NOT NULL,request_id text NOT NULL,request_digest bytea NOT NULL,username text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(user_id,request_id));`
}

type usernameChangeRequest struct {
	Username         string `json:"username"`
	ExpectedUsername string `json:"expectedUsername"`
	RequestID        string `json:"requestId"`
}

func (a *App) changeOwnUsername(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q usernameChangeRequest
	if err = decode(r, &q); err != nil {
		return err
	}
	if !validReceiptNonce(q.RequestID) {
		return fail(400, "REQUEST_ID_REQUIRED", "A stable original requestId is required")
	}
	if q.Username, err = normalizeUsername(q.Username); err != nil {
		return err
	}
	if q.ExpectedUsername, err = normalizeUsername(q.ExpectedUsername); err != nil {
		return err
	}
	reserved := map[string]bool{"admin": true, "administrator": true, "root": true, "system": true, "jobsim_admin": true}
	if reserved[q.Username] && q.Username != u.Username {
		return fail(400, "RESERVED_USERNAME", "Username reserved for controlled initialization")
	}
	raw, _ := json.Marshal(q)
	digest := keyedDigest(a.Config.Key, "own-username:"+u.ID, raw)
	var prior []byte
	err = a.DB.QueryRow(r.Context(), `SELECT request_digest FROM auth.username_operations WHERE user_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&prior)
	if err == nil {
		if !bytes.Equal(prior, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original username operation differs")
		}
		return a.respondUsernameChange(w, r, u, q)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if err = a.rate(r.Context(), "username:"+u.ID, 5); err != nil {
		return err
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "username:"+q.Username); err != nil {
		return err
	}
	var current string
	if err = tx.QueryRow(r.Context(), `SELECT username FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' AND pending_operation_id IS NULL FOR UPDATE`, u.ID).Scan(&current); err != nil {
		return fail(409, "USERNAME_CONTEXT_CHANGED", "Original active account is unavailable")
	}
	// A concurrent exact retry must read its original result before checking the old expectation.
	err = tx.QueryRow(r.Context(), `SELECT request_digest FROM auth.username_operations WHERE user_id=$1 AND request_id=$2`, u.ID, q.RequestID).Scan(&prior)
	if err == nil {
		if !bytes.Equal(prior, digest) {
			return fail(409, "IDEMPOTENCY_CONFLICT", "Original username operation differs")
		}
		if err = tx.Commit(r.Context()); err != nil {
			return err
		}
		return a.respondUsernameChange(w, r, u, q)
	}
	if err != pgx.ErrNoRows {
		return err
	}
	if current != q.ExpectedUsername {
		return fail(409, "USERNAME_CHANGED", "Username changed; reload before a new explicit edit")
	}
	var occupied bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.users WHERE username=$1 AND id<>$2) OR EXISTS(SELECT 1 FROM auth.identity_reservations WHERE username=$1 AND user_id<>$2)`, q.Username, u.ID).Scan(&occupied); err != nil {
		return err
	}
	if occupied {
		return fail(409, "USERNAME_TAKEN", "Username is already reserved")
	}
	operation := "username:" + u.ID + ":" + q.RequestID
	payload := map[string]any{"userId": u.ID, "username": q.Username}
	// The durable reservation prevents another identity claiming this name after restore.
	if err = a.protect(r.Context(), operation, "intent", "auth.reserve_username", payload); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO auth.identity_reservations(username,user_id) VALUES($1,$3),($2,$3) ON CONFLICT DO NOTHING`, current, q.Username, u.ID); err != nil {
		return err
	}
	var reservedOwner string
	if err = tx.QueryRow(r.Context(), `SELECT user_id::text FROM auth.identity_reservations WHERE username=$1`, q.Username).Scan(&reservedOwner); err != nil {
		return err
	}
	if reservedOwner != u.ID {
		return fail(409, "USERNAME_TAKEN", "Username is already reserved")
	}
	if _, err = tx.Exec(r.Context(), `UPDATE auth.users SET username=$2,row_version=row_version+1 WHERE id=$1`, u.ID, q.Username); err != nil {
		var duplicate *pgconn.PgError
		if errors.As(err, &duplicate) && duplicate.Code == "23505" {
			return fail(409, "USERNAME_TAKEN", "Username is already reserved")
		}
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO auth.username_operations(user_id,request_id,request_digest,username) VALUES($1,$2,$3,$4)`, u.ID, q.RequestID, digest, q.Username); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	if err = a.protect(r.Context(), operation, "result", "auth.reserve_username", payload); err != nil {
		return err
	}
	return a.respondUsernameChange(w, r, u, q)
}

func (a *App) respondUsernameChange(w http.ResponseWriter, r *http.Request, u User, q usernameChangeRequest) error {
	if err := a.DB.QueryRow(r.Context(), `SELECT username FROM auth.users WHERE id=$1`, u.ID).Scan(&u.Username); err != nil {
		return err
	}
	var profile AvatarProfile
	if err := a.rpc(r.Context(), "directory", "/internal/profile", map[string]string{"userId": u.ID}, &profile); err != nil {
		return err
	}
	applyUserProfile(&u, profile)
	respond(w, 200, map[string]any{"user": u, "current": u.Username == q.Username, "username": q.Username, "requestId": q.RequestID})
	return nil
}
