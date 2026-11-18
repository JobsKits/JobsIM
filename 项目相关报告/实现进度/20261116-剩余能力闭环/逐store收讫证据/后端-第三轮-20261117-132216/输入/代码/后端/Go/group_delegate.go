package jobsim

import (
	"context"
	"net/http"
)

type groupIssuer struct {
	Active   bool   `json:"active"`
	Security string `json:"securityVersion"`
	Epoch    string `json:"restoreEpoch"`
}

func (a *App) groupIssuerRoutes(h func(string, func(http.ResponseWriter, *http.Request) error), internal func(func(http.ResponseWriter, *http.Request) error) func(http.ResponseWriter, *http.Request) error) {
	h("POST /internal/groups/issuer", internal(func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("X-JobsIM-Caller") != "conversation" {
			return fail(403, "GROUP_OWNER_REQUIRED", "Only the group owner service may verify its delegation")
		}
		var q struct {
			User string `json:"userId"`
		}
		if err := decode(r, &q); err != nil {
			return err
		}
		if !validObjectID(q.User) {
			return fail(400, "INVALID_GROUP_ISSUER", "Original issuer identity required")
		}
		epoch, err := a.currentIdentityEpoch(r.Context())
		if err != nil {
			return err
		}
		result := groupIssuer{Epoch: str(epoch)}
		err = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM auth.users WHERE id=$1 AND state='active' AND recovery_state='normal' AND deletion_state='none' AND pending_operation_id IS NULL),COALESCE((SELECT security_version::text FROM auth.users WHERE id=$1),'0')`, q.User).Scan(&result.Active, &result.Security)
		if err != nil {
			return err
		}
		respond(w, 200, result)
		return nil
	}))
}

func (a *App) groupInvitationIssuer(ctx context.Context, user string) (groupIssuer, error) {
	var result groupIssuer
	err := a.rpc(ctx, "identity", "/internal/groups/issuer", map[string]string{"userId": user}, &result)
	return result, err
}
