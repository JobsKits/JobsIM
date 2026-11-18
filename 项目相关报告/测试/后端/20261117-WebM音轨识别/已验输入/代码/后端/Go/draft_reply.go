package jobsim

import (
	"context"
	"crypto/hmac"
)

// Draft replies point into the same conversation. The proof freezes the reader
// qualification that created this intent; a later grant cannot repair it.
func (a *App) prepareDraftReply(ctx context.Context, user, conversation, id, version, proof string) (Message, error) {
	var empty Message
	if !syncValidationID(conversation) || !syncValidationID(id) || !syncValidationCounter(version, false) {
		return empty, fail(400, "INVALID_DRAFT_REPLY", "Original conversation, source identity and exact content version required")
	}
	source, err := a.loadMessage(ctx, id, user)
	if err != nil {
		return empty, err
	}
	if source.ConversationID != conversation {
		return empty, fail(403, "DRAFT_REPLY_NOT_VISIBLE", "Draft replies must remain inside their original conversation")
	}
	if source.forwardUncertain {
		return empty, fail(503, "DRAFT_REPLY_QUALIFICATION_UNAVAILABLE", "Current reply source qualification cannot be proved")
	}
	if source.Deleted || source.ContentUnavailable || source.ForwardUnavailable || source.sourcePending {
		return empty, fail(409, "DRAFT_REPLY_SOURCE_UNAVAILABLE", "Original reply source is no longer available")
	}
	if source.ContentVersion != version {
		return empty, fail(409, "DRAFT_REPLY_SOURCE_CHANGED", "Original reply source content version changed")
	}
	if proof == "" || source.ReplyAdmission == "" || !hmac.Equal([]byte(proof), []byte(source.ReplyAdmission)) {
		return empty, fail(409, "DRAFT_REPLY_ADMISSION_CHANGED", "Original reply reader admission is missing or no longer current")
	}
	return source, nil
}
