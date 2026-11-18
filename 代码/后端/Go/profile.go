// profile.go
// JobsIM
// Created by Jobs on 2026年11月18日，星期三.
package jobsim

import (
	"context"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type ContactBinding struct {
	State string `json:"state"`
	Value string `json:"value,omitempty"`
}

type ContactBindings struct {
	Email ContactBinding `json:"email"`
	Phone ContactBinding `json:"phone"`
}

func init() {
	migrations["directory"] += `ALTER TABLE directory.profiles ADD COLUMN IF NOT EXISTS signature text NOT NULL DEFAULT '';`
	migrations["identity"] += `CREATE TABLE IF NOT EXISTS auth.contact_bindings(user_id uuid NOT NULL REFERENCES auth.users(id),kind text NOT NULL CHECK(kind IN('email','phone')),value_ciphertext bytea NOT NULL,verified_at timestamptz NOT NULL,PRIMARY KEY(user_id,kind));`
}

func (a *App) profileIdentityRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	h("GET /api/users/{id}/profile", a.userProfile)
}

func (a *App) ownContactBindings(ctx context.Context, user string) (*ContactBindings, error) {
	v := &ContactBindings{Email: ContactBinding{State: "unbound"}, Phone: ContactBinding{State: "unbound"}}
	rows, err := a.DB.Query(ctx, `SELECT kind,value_ciphertext FROM auth.contact_bindings WHERE user_id=$1`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var encrypted []byte
		if err = rows.Scan(&kind, &encrypted); err != nil {
			return nil, err
		}
		value, readErr := a.unseal(encrypted, "profile-contact:"+a.Config.DeploymentID+":"+user+":"+kind)
		binding := ContactBinding{State: "unavailable"}
		if readErr == nil && value != "" && utf8.ValidString(value) {
			binding = ContactBinding{State: "bound", Value: value}
		}
		if kind == "email" {
			v.Email = binding
		} else if kind == "phone" {
			v.Phone = binding
		}
	}
	return v, rows.Err()
}

func hiddenContactBindings() *ContactBindings {
	return &ContactBindings{Email: ContactBinding{State: "hidden"}, Phone: ContactBinding{State: "hidden"}}
}

func (a *App) userProfile(w http.ResponseWriter, r *http.Request) error {
	viewer, err := a.current(r)
	if err != nil {
		return err
	}
	id := r.PathValue("id")
	if !validAssetID(id) {
		return fail(400, "INVALID_PROFILE_ID", "Stable user ID required")
	}
	var u User
	var registered time.Time
	if err = a.DB.QueryRow(r.Context(), `SELECT id::text,username,display_name,created_at FROM auth.users WHERE id=$1 AND state='active' AND deletion_state='none'`, id).Scan(&u.ID, &u.Username, &u.DisplayName, &registered); err != nil {
		return fail(404, "PROFILE_UNAVAILABLE", "Current profile unavailable")
	}
	if viewer.ID != id {
		if err = a.rpc(r.Context(), "directory", "/internal/profile/visible", map[string]string{"ownerId": id, "viewerId": viewer.ID}, nil); err != nil {
			return err
		}
	}
	var profile AvatarProfile
	if err = a.rpc(r.Context(), "directory", "/internal/profile", map[string]string{"userId": id}, &profile); err != nil {
		return err
	}
	u.CreatedAt = registered.UTC().Format(time.RFC3339Nano)
	applyUserProfile(&u, profile)
	u.ContactBindings = hiddenContactBindings()
	if viewer.ID == id {
		u.ContactBindings, err = a.ownContactBindings(r.Context(), id)
		if err != nil {
			return err
		}
	}
	respond(w, 200, u)
	return nil
}

func (a *App) profileDirectoryRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/profile/visible", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "identity" {
			return fail(403, "PROFILE_OWNER_REQUIRED", "Identity owner required")
		}
		var q struct {
			Owner  string `json:"ownerId"`
			Viewer string `json:"viewerId"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		var blocked, friends bool
		if err := a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM directory.blocks WHERE(owner_id=$1 AND peer_id=$2)OR(owner_id=$2 AND peer_id=$1)),EXISTS(SELECT 1 FROM directory.contacts WHERE owner_id=$1 AND peer_id=$2 AND state='friend')`, q.Owner, q.Viewer).Scan(&blocked, &friends); err != nil {
			return err
		}
		if blocked {
			return fail(403, "PROFILE_FORBIDDEN", "Current profile visibility required")
		}
		if !friends {
			var common struct {
				Visible bool `json:"visible"`
			}
			if err := a.rpc(r.Context(), "conversation", "/internal/profile-visible", map[string]string{"first": q.Owner, "second": q.Viewer}, &common); err != nil {
				return err
			}
			if !common.Visible {
				return fail(403, "PROFILE_FORBIDDEN", "Current profile visibility required")
			}
		}
		respond(w, 200, map[string]bool{"allowed": true})
		return nil
	}))
}

func (a *App) patchOwnProfile(w http.ResponseWriter, r *http.Request) error {
	u, err := a.current(r)
	if err != nil {
		return err
	}
	var q struct {
		DisplayName *string `json:"displayName"`
		Signature   *string `json:"signature"`
	}
	if err = decode(r, &q); err != nil {
		return err
	}
	if q.DisplayName == nil && q.Signature == nil {
		return fail(400, "PROFILE_FIELD_REQUIRED", "An explicit profile field required")
	}
	if q.DisplayName != nil && (!utf8.ValidString(*q.DisplayName) || utf8.RuneCountInString(*q.DisplayName) < 1 || utf8.RuneCountInString(*q.DisplayName) > 64) {
		return fail(400, "DISPLAY_NAME_LIMIT", "Display name requires1–64 Unicode characters")
	}
	if q.Signature != nil && (!utf8.ValidString(*q.Signature) || utf8.RuneCountInString(*q.Signature) > 200) {
		return fail(400, "SIGNATURE_LIMIT", "Signature permits at most200 Unicode characters")
	}
	name := u.DisplayName
	if q.DisplayName != nil {
		name = *q.DisplayName
	}
	_, err = a.DB.Exec(r.Context(), `INSERT INTO directory.profiles(user_id,display_name,signature) VALUES($1,$2,COALESCE($3,'')) ON CONFLICT(user_id) DO UPDATE SET display_name=CASE WHEN $4 THEN EXCLUDED.display_name ELSE directory.profiles.display_name END,signature=COALESCE($3,directory.profiles.signature),row_version=directory.profiles.row_version+1,updated_at=now()`, u.ID, name, q.Signature, q.DisplayName != nil)
	if err != nil {
		return err
	}
	profile, err := a.avatarProfileFields(r.Context(), u.ID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	applyUserProfile(&u, profile)
	respond(w, 200, u)
	return nil
}
