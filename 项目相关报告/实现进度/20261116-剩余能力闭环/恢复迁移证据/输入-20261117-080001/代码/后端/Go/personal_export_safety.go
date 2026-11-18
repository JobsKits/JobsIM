package jobsim

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func personalExportFailureOperation(err error) string {
	code := "EXPORT_BUILD_FAILED"
	var cause *apiError
	if errors.As(err, &cause) && len(cause.Code) > 0 && len(cause.Code) <= 64 {
		valid := true
		for _, letter := range cause.Code {
			valid = valid && (letter >= 'A' && letter <= 'Z' || letter >= '0' && letter <= '9' || letter == '_')
		}
		if valid {
			code = cause.Code
		}
	}
	return "builder-failed:" + code
}

func personalExportJournalKinds() map[string][]string {
	kinds := map[string][]string{}
	for _, owner := range personalExportOwners() {
		kinds[owner+".personal_export_holder"] = []string{"jobId", "artifactId", "userId", "restoreEpoch", "buildGeneration", "owner", "kind", "objectId", "sourceRevision", "revisionEpoch", "expiresAt"}
	}
	kinds["media.personal_export_avatar_holder"] = []string{"jobId", "artifactId", "userId", "objectId", "restoreEpoch", "buildGeneration", "expiresAt"}
	return kinds
}

func validatePersonalExportJournalField(field string, raw any) (bool, error) {
	value, ok := raw.(string)
	valid := false
	switch field {
	case "owner":
		valid = ok && (value == "auth" || value == "directory" || value == "conversation" || value == "sync" || value == "governance")
	case "kind":
		valid = ok && personalExportRefOwner(value) != "" && value != "message"
	case "sourceRevision":
		valid = ok && syncValidationCounter(value, true)
	case "revisionEpoch":
		valid = ok && syncValidationCounter(value, false)
	default:
		return false, nil
	}
	if !valid {
		return true, fail(400, "INVALID_JOURNAL_PAYLOAD", "Invalid personal export safety field")
	}
	return true, nil
}

func ReplayPersonalExportSafety(ctx context.Context, business *pgx.Conn, record journalRecord) (bool, error) {
	if !strings.HasSuffix(record.Kind, ".personal_export_holder") && record.Kind != "media.personal_export_avatar_holder" {
		return false, nil
	}
	// Identical denial-only reconstruction to existing message/media holders.
	// A source body, ZIP key, download ticket or active grant is never replayed.
	copy := record
	copy.Kind = "message.export_holder"
	return ReplayExportSafety(ctx, business, copy)
}
