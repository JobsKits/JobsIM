package jobsim

import "context"

// The committed owner floor takes effect before the bounded byte cleanup.
func (a *App) requireMediaOwnerReadable(ctx context.Context, object string) error {
	var erased bool
	if err := a.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media.account_erasure_floors f WHERE f.user_id=o.owner_id) FROM media.objects o WHERE o.id=$1`, object).Scan(&erased); err != nil {
		return fail(503, "MEDIA_OWNER_STATE_UNAVAILABLE", "Current original media owner state is required")
	}
	if erased {
		return fail(410, "MEDIA_OWNER_ERASED", "An account erasure floor permanently denies original media")
	}
	return nil
}
