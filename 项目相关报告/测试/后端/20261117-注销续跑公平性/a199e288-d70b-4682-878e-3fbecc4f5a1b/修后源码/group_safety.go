package jobsim

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

func groupJournalKinds() map[string][]string {
	base := []string{"conversationId", "userId", "version"}
	fields := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	return map[string][]string{
		"conversation.group_policy":          fields("groupMuteAll", "groupSlowSeconds", "groupMaxMembers"),
		"conversation.group_member":          fields("targetId", "membershipGeneration", "groupRoleGeneration", "groupRole", "groupMutedUntil"),
		"conversation.group_dissolve":        fields(),
		"conversation.group_transfer":        fields("ownerId", "targetId", "membershipGeneration"),
		"conversation.group_recover":         fields("ownerId", "targetId"),
		"conversation.group_recovery_revoke": {"userId", "requestKey"},
		"conversation.group_invite_revoke":   {"conversationId", "userId", "inviteId"},
		"conversation.group_invite_use":      {"conversationId", "userId", "inviteId", "groupInviteUses"},
	}
}

func validateGroupJournalField(field string, raw any) (bool, error) {
	value, ok := raw.(string)
	valid := false
	switch field {
	case "groupMuteAll":
		valid = ok && (value == "true" || value == "false")
	case "groupSlowSeconds", "groupMaxMembers", "groupInviteUses":
		n, e := strconv.Atoi(value)
		valid = ok && e == nil && strconv.Itoa(n) == value
		if field == "groupSlowSeconds" {
			valid = valid && n >= 0 && n <= 3600
		} else if field == "groupMaxMembers" {
			valid = valid && n >= 2 && n <= 10000
		} else {
			valid = valid && n >= 1 && n <= 10000
		}
	case "groupRoleGeneration":
		valid = ok && syncValidationCounter(value, false)
	case "groupRole":
		valid = ok && (value == "admin" || value == "member")
	case "groupMutedUntil":
		_, e := time.Parse(time.RFC3339Nano, value)
		valid = ok && (value == "" || e == nil)
	default:
		return false, nil
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid minimal group safety field")
	}
	return true, nil
}

// Restore only reconstructs restrictions. An uncertain grant, unmute or recovery
// never promotes a historical member; independent governance must review it.
func ReplayGroupSafety(ctx context.Context, business *pgx.Conn, q journalRecord) (bool, error) {
	if _, exists := groupJournalKinds()[q.Kind]; !exists {
		return false, nil
	}
	if q.Owner != "conversation" {
		return true, fmt.Errorf("group safety owner differs")
	}
	text := func(key string) string { return stringValue(q.Payload, key) }
	tx, err := business.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	switch q.Kind {
	case "conversation.group_recovery_revoke":
		_, err = tx.Exec(ctx, `DELETE FROM conversation.governance_grants WHERE user_id=$1`, text("userId"))
	case "conversation.group_policy":
		_, err = tx.Exec(ctx, `INSERT INTO conversation.group_settings(conversation_id,policy_version,mute_all,slow_seconds,max_members) SELECT id,$2,$3,$4,$5 FROM conversation.conversations WHERE id=$1 ON CONFLICT(conversation_id) DO UPDATE SET policy_version=GREATEST(conversation.group_settings.policy_version,EXCLUDED.policy_version),mute_all=conversation.group_settings.mute_all OR EXCLUDED.mute_all,slow_seconds=GREATEST(conversation.group_settings.slow_seconds,EXCLUDED.slow_seconds),max_members=LEAST(conversation.group_settings.max_members,EXCLUDED.max_members)`, text("conversationId"), text("version"), text("groupMuteAll"), text("groupSlowSeconds"), text("groupMaxMembers"))
	case "conversation.group_member":
		_, err = tx.Exec(ctx, `UPDATE conversation.members SET role=CASE WHEN $4='member' THEN 'member' ELSE role END,role_generation=GREATEST(role_generation,$5::bigint) WHERE conversation_id=$1 AND user_id=$2 AND membership_generation=$3::bigint AND role<>'owner'`, text("conversationId"), text("targetId"), text("membershipGeneration"), text("groupRole"), text("groupRoleGeneration"))
		if err == nil && text("groupMutedUntil") != "" {
			_, err = tx.Exec(ctx, `INSERT INTO conversation.group_mutes(conversation_id,user_id,membership_generation,muted_until) SELECT conversation_id,user_id,membership_generation,$4 FROM conversation.members WHERE conversation_id=$1 AND user_id=$2 AND membership_generation=$3::bigint ON CONFLICT(conversation_id,user_id) DO UPDATE SET membership_generation=EXCLUDED.membership_generation,muted_until=CASE WHEN conversation.group_mutes.membership_generation=EXCLUDED.membership_generation THEN GREATEST(conversation.group_mutes.muted_until,EXCLUDED.muted_until) ELSE EXCLUDED.muted_until END`, text("conversationId"), text("targetId"), text("membershipGeneration"), text("groupMutedUntil"))
		}
	case "conversation.group_dissolve":
		_, err = tx.Exec(ctx, `UPDATE conversation.conversations SET state='dissolved' WHERE id=$1 AND kind='group'`, text("conversationId"))
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE conversation.group_invites SET revoked_at=COALESCE(revoked_at,now()) WHERE conversation_id=$1`, text("conversationId"))
		}
	case "conversation.group_recover", "conversation.group_transfer":
		_, err = tx.Exec(ctx, `UPDATE conversation.conversations SET state='owner_unavailable' WHERE id=$1 AND owner_id=$2 AND state='active' AND kind='group'`, text("conversationId"), text("ownerId"))
	case "conversation.group_invite_revoke":
		_, err = tx.Exec(ctx, `UPDATE conversation.group_invites SET revoked_at=COALESCE(revoked_at,now()) WHERE conversation_id=$1 AND id=$2`, text("conversationId"), text("inviteId"))
	case "conversation.group_invite_use":
		_, err = tx.Exec(ctx, `UPDATE conversation.group_invites SET uses=GREATEST(uses,$3::integer) WHERE conversation_id=$1 AND id=$2`, text("conversationId"), text("inviteId"), text("groupInviteUses"))
	}
	if err != nil {
		return true, err
	}
	if text("version") != "" {
		_, err = tx.Exec(ctx, `UPDATE conversation.conversations SET version=GREATEST(version,$2::bigint),updated_at=now() WHERE id=$1`, text("conversationId"), text("version"))
		if err != nil {
			return true, err
		}
	}
	return true, tx.Commit(ctx)
}
