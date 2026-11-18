package jobsim

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Retained personal progress is metadata only. It never restores cloud read or
// makes an observation from a former membership apply to the current one.
func (a *App) syncPersonalReadProjection(ctx context.Context, user string, ref syncRef) (any, error) {
	var epoch string
	if err := a.DB.QueryRow(ctx, `SELECT restore_epoch::text FROM sync.instance WHERE singleton AND deployment_id=$1`, a.Config.DeploymentID).Scan(&epoch); err != nil {
		return nil, err
	}
	u := User{ID: user, RestoreEpoch: epoch}
	metadata, memberError := a.member(ctx, user, ref.ConversationID)
	var membership readPointMembership
	if memberError == nil {
		var err error
		membership, err = readPointMembershipFromMetadata(metadata)
		if err != nil {
			return nil, err
		}
	} else {
		var rejection *apiError
		if !errors.As(memberError, &rejection) || rejection.Status != 403 {
			return nil, memberError
		}
	}
	if ref.Kind == "read_state" {
		if ref.ID != stableID(a.Config.Key, "self-read-cursor:"+a.Config.DeploymentID+":personal:"+user+":"+ref.ConversationID) {
			return nil, fail(403, "FORBIDDEN", "Personal read identity does not match this principal")
		}
		state, err := a.loadSelfReadState(ctx, a.DB, u, ref.ConversationID)
		if err != nil {
			return nil, err
		}
		if memberError != nil {
			if !state.Exists {
				if ref.State == "removed" {
					return map[string]any{"id": ref.ID, "removed": true, "scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "userId": user, "conversationId": ref.ConversationID, "revisionEpoch": epoch}, nil
				}
				return nil, memberError
			}
			membership.Generation = state.MembershipGeneration
		}
		view := a.selfReadView(u, ref.ConversationID, membership, state)
		if memberError != nil {
			view.Writable = false
			view.ContinuityStatus = "membership_unavailable"
		}
		return view, nil
	}
	var conversation, generation, message, observedEpoch, version string
	var creationEpoch, creationSequence *int64
	var known, eligible bool
	var observed time.Time
	err := a.DB.QueryRow(ctx, `SELECT conversation_id::text,membership_generation::text,message_id::text,revision_epoch::text,row_version::text,creation_epoch,creation_sequence,order_known,eligible,observed_at FROM sync.read_observations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND id=$3`, a.Config.DeploymentID, user, ref.ID).Scan(&conversation, &generation, &message, &observedEpoch, &version, &creationEpoch, &creationSequence, &known, &eligible, &observed)
	if err == pgx.ErrNoRows && ref.State == "removed" {
		return map[string]any{"id": ref.ID, "removed": true, "scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "userId": user, "conversationId": ref.ConversationID, "revisionEpoch": epoch}, nil
	}
	if err != nil {
		return nil, err
	}
	if conversation != ref.ConversationID {
		return nil, fail(403, "FORBIDDEN", "Observation belongs to another conversation")
	}
	if observedEpoch != epoch {
		version = "0"
	}
	point := map[string]any{"id": ref.ID, "scopeId": a.Config.DeploymentID, "sourceContextId": "personal", "userId": user, "conversationId": conversation, "messageId": message, "membershipGeneration": generation, "revisionEpoch": epoch, "version": version, "observedInEpoch": observedEpoch, "observedAt": observed, "orderKnown": known, "eligibleAtObservation": eligible, "currentlyApplicable": memberError == nil && membership.Generation == generation, "countStatus": "not_computed", "creationEpoch": "unknown", "creationSequence": "unknown"}
	if known && creationEpoch != nil && creationSequence != nil {
		point["creationEpoch"], point["creationSequence"] = str(*creationEpoch), str(*creationSequence)
	}
	return point, nil
}
