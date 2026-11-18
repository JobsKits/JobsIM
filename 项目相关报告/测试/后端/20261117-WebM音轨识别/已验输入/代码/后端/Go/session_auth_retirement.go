package jobsim

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func authenticationPredecessors(q registerRequest, recovering bool) ([]string, error) {
	if !recovering {
		if q.PreviousFamilyID != "" || q.PreviousRequestID != "" || len(q.PreviousRequestIDs) > 0 {
			return nil, fail(400, "INVALID_PREVIOUS_AUTHENTICATION", "Predecessors require controlled reauthentication")
		}
		return nil, nil
	}
	if q.PreviousFamilyID != "" && !validObjectID(q.PreviousFamilyID) {
		return nil, fail(400, "INVALID_PREVIOUS_FAMILY", "Original family must be a valid identifier")
	}
	items := append([]string{}, q.PreviousRequestIDs...)
	seen := map[string]bool{}
	for _, key := range items {
		if seen[key] {
			return nil, fail(400, "INVALID_PREVIOUS_AUTHENTICATION", "Predecessor closure must not contain duplicates")
		}
		seen[key] = true
	}
	if q.PreviousRequestID != "" {
		if seen[q.PreviousRequestID] && (len(items) == 0 || items[len(items)-1] != q.PreviousRequestID) {
			return nil, fail(400, "INVALID_PREVIOUS_AUTHENTICATION", "Scalar predecessor must be the closure leaf")
		}
		if !seen[q.PreviousRequestID] {
			items = append(items, q.PreviousRequestID)
		}
	}
	if len(items) > 64 || len(items) == 0 && q.PreviousFamilyID == "" {
		return nil, fail(400, "INVALID_PREVIOUS_AUTHENTICATION", "Bounded predecessor closure or original family required")
	}
	for _, key := range items {
		if len(key) < 8 || len(key) > 128 || key == q.RequestID {
			return nil, fail(400, "INVALID_PREVIOUS_AUTHENTICATION", "Original predecessor keys and a distinct new intent required")
		}
	}
	return items, nil
}

// Caller holds the verified principal row lock. Collect every known family even
// when an older retirement edge survived only in the independent journal.
func (a *App) retireAuthenticationClosure(ctx context.Context, tx pgx.Tx, user, current string, keys []string, sessions map[string]bool) error {
	positions := map[string]int{}
	for i, key := range keys {
		positions[key] = i
	}
	for index, key := range keys {
		var retiredBy string
		e := tx.QueryRow(ctx, `SELECT retired_by_request_id FROM auth.authentication_retirements WHERE user_id=$1 AND request_id=$2`, user, key).Scan(&retiredBy)
		if e == pgx.ErrNoRows {
			retiredBy, e = a.authenticationRetirement(ctx, user, key)
		}
		if e != nil {
			return e
		}
		if retiredBy != "" && retiredBy != current {
			next, covered := positions[retiredBy]
			if !covered || next <= index {
				return fail(409, "PREVIOUS_AUTHENTICATION_SUPERSEDED", "Retain and include the later recorded successor before recovering this closure")
			}
		}
		var owner, purpose string
		e = tx.QueryRow(ctx, `SELECT user_id::text,purpose FROM auth.local_auth_requests WHERE request_id=$1`, key).Scan(&owner, &purpose)
		if e == nil {
			if owner != user || (purpose != "login" && purpose != "registration" && purpose != "reauthentication") {
				return fail(403, "PREVIOUS_AUTHENTICATION_INVALID", "All original authentications must belong to this verified account and instance")
			}
			var id string
			if e = tx.QueryRow(ctx, `SELECT s.id::text FROM auth.local_auth_requests a JOIN auth.sessions s ON s.id=a.session_id AND s.user_id=a.user_id AND s.family_id IS NOT DISTINCT FROM a.family_id WHERE a.request_id=$1 AND a.user_id=$2 AND s.source_context_id='personal' FOR UPDATE OF s`, key, user).Scan(&id); e != nil {
				return fail(503, "PREVIOUS_AUTHENTICATION_UNRESOLVED", "Original session mapping cannot be proved; retain the complete recovery closure")
			}
			sessions[id] = true
		} else if e != pgx.ErrNoRows {
			return e
		}
		if retiredBy == "" {
			retiredBy = current
			if e = a.protect(ctx, authenticationRetirementOperation(user, key), "intent", "auth.retire_authentication", map[string]any{"userId": user, "requestKey": key, "replacementKey": current}); e != nil {
				return e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO auth.authentication_retirements(user_id,request_id,retired_by_request_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, user, key, retiredBy); e != nil {
			return e
		}
	}
	return nil
}
