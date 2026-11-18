// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
)

type receiptQualifiedMigrationSource struct {
	Envelope         string
	BodyDigest       string
	ContentReceiptID string
	ObjectReceiptID  string
	Object           *receiptLocalObject
	Original         *os.File
}

// The caller owns Original.Close. This reads an existing qualified fact, never
// reconstructs an envelope, creates an ACK or imports source qualification.
func (c *client) localQualifiedMigrationMessage(messageID, contentVersion string) (receiptQualifiedMigrationSource, error) {
	var result receiptQualifiedMigrationSource
	if !syncUUIDPattern.MatchString(messageID) {
		return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	if n, err := syncCounter(contentVersion); err != nil || n == 0 {
		return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	s, current, err := c.receiptContext()
	if err != nil {
		return result, err
	}
	control, err := s.receiptControl(s.db, nil, current, true)
	if err != nil {
		return result, err
	}
	_, err = c.getReceiptNegotiation(control)
	if err != nil {
		return result, err
	}
	var terminal int
	if err = s.db.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", messageID).Scan(&terminal); err != nil || terminal != 0 {
		return result, syncRefuse("MIGRATION_GLOBAL_SOURCE")
	}
	var id string
	err = s.db.QueryRow(`SELECT b.request_id FROM receipt_bodies b JOIN receipt_outbox o ON o.id=b.request_id WHERE b.epoch=? AND b.message_id=? AND b.content_version=? AND o.state='completed' AND json_extract(o.payload,'$.kind')='message_content' ORDER BY b.rowid DESC LIMIT 1`, s.epoch, messageID, contentVersion).Scan(&id)
	if err != nil {
		return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	parent, err := receiptParent(s.db, id, control.syncBinding)
	if err != nil || parent.Message != messageID || parent.Content != contentVersion {
		return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	var recorded json.RawMessage
	if err = c.call("POST", "/api/sync/receipt-results", json.RawMessage(parent.Body), &recorded); err != nil {
		return result, err
	}
	if err = receiptValidateResult(recorded, parent); err != nil {
		return result, err
	}
	generation, manifest, err := receiptOriginalDelivery(parent)
	if err != nil {
		return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	var target receiptTargetView
	if generation == "" {
		if err = c.receiptLegacyMigrationCurrent(control, parent); err != nil {
			return result, err
		}
	} else {
		qualification := receiptParentQualification(parent)
		path := syncSchemaPath("/api/sync/receipt-targets/"+url.PathEscape(messageID)+"?qualificationId="+url.QueryEscape(qualification), control.schema())
		if err = c.call("GET", path, nil, &target); err != nil {
			return result, err
		}
		if receiptValidateTarget(target, messageID, s.epoch) != nil || target.Qualification != qualification || !receiptBool(target.MessageNow, true) || !receiptBool(target.ContentACK, true) || target.Generation != generation || target.Digest != manifest || target.Content != contentVersion {
			return result, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
		}
	}
	if err = s.db.QueryRow("SELECT body_envelope,body_digest FROM receipt_bodies WHERE request_id=? AND epoch=? AND message_id=? AND content_version=?", id, s.epoch, messageID, contentVersion).Scan(&result.Envelope, &result.BodyDigest); err != nil || result.BodyDigest != parent.Digest {
		return receiptQualifiedMigrationSource{}, syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	raw, err := syncEnvelope(result.Envelope, result.BodyDigest)
	if err != nil {
		return receiptQualifiedMigrationSource{}, err
	}
	normalized, err := receiptNormalized(raw)
	if err != nil || fingerprint(string(normalized)) != parent.CachedDigest {
		return receiptQualifiedMigrationSource{}, syncRefuse("MIGRATION_SOURCE_CHANGED")
	}
	var message struct {
		ID          string `json:"id"`
		Content     string `json:"contentVersion"`
		Attachment  string `json:"attachmentId"`
		Deleted     bool   `json:"deleted"`
		Unavailable bool   `json:"contentUnavailable"`
		Forwarded   bool   `json:"forwarded"`
	}
	if json.Unmarshal(raw, &message) != nil || message.ID != messageID || message.Content != contentVersion || message.Deleted || message.Unavailable || message.Forwarded {
		return receiptQualifiedMigrationSource{}, syncRefuse("MIGRATION_SOURCE_CHANGED")
	}
	result.ContentReceiptID = id
	if message.Attachment != "" && (generation == "" || receiptBool(target.ObjectACK, true)) {
		var objectID string
		err = s.db.QueryRow(`SELECT o.id FROM receipt_outbox o JOIN receipt_objects f ON f.request_id=o.id WHERE o.epoch=? AND o.message_id=? AND o.state='completed' AND f.state='ready' AND json_extract(o.payload,'$.kind')='object_received' AND json_extract(o.payload,'$.object.parentReceiptId')=? ORDER BY o.rowid DESC LIMIT 1`, s.epoch, messageID, id).Scan(&objectID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return receiptQualifiedMigrationSource{}, err
		}
		if err == nil {
			o, loadErr := s.loadReceipt(objectID)
			if loadErr != nil || o.Object == nil || o.Binding.check(control.syncBinding, false) != nil || o.Content != contentVersion || o.Object.Object != message.Attachment || o.Object.Parent != id || receiptValidateObjectResult(o.Result, o) != nil || s.checkReceiptObjectDescriptor(o) != nil {
				return receiptQualifiedMigrationSource{}, syncRefuse("MIGRATION_ORIGINAL_OBJECT_CHANGED")
			}
			var recordedObject json.RawMessage
			if err = c.call("POST", "/api/sync/object-receipt-results", json.RawMessage(o.Body), &recordedObject); err != nil {
				return receiptQualifiedMigrationSource{}, err
			}
			if err = receiptValidateObjectResult(recordedObject, o); err != nil {
				return receiptQualifiedMigrationSource{}, err
			}
			file, openErr := s.openReceiptObject(*o.Object)
			if openErr != nil {
				return receiptQualifiedMigrationSource{}, openErr
			}
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				file.Close()
				return receiptQualifiedMigrationSource{}, err
			}
			result.ObjectReceiptID, result.Object, result.Original = objectID, o.Object, file
		}
	}
	if err = c.receiptNegotiationCurrent(control); err != nil {
		if result.Original != nil {
			result.Original.Close()
		}
		return receiptQualifiedMigrationSource{}, err
	}
	if err = s.db.QueryRow("SELECT count(*) FROM sync_terminal WHERE id=?", messageID).Scan(&terminal); err != nil || terminal != 0 {
		if result.Original != nil {
			result.Original.Close()
		}
		return receiptQualifiedMigrationSource{}, syncRefuse("MIGRATION_GLOBAL_SOURCE")
	}
	return result, nil
}

func (c *client) receiptLegacyMigrationCurrent(control syncControl, parent receiptOperation) error {
	var body struct {
		Challenge string `json:"challenge"`
	}
	if json.Unmarshal([]byte(parent.Body), &body) != nil {
		return syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	parts := strings.Split(body.Challenge, ".")
	if len(parts) != 3 || parts[0] != "rc1" {
		return syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	var original receiptProofTuple
	if err != nil || syncDecode(raw, &original) != nil || original.Permission == "" {
		return syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	var view struct {
		syncBinding
		Permission    string `json:"permissionGeneration"`
		Qualification string `json:"qualification"`
		Qualified     *bool  `json:"qualifiedAck"`
		After         *bool  `json:"afterReceiptEnabled"`
	}
	if err = c.call("GET", syncSchemaPath("/api/sync/context", control.schema()), nil, &view); err != nil {
		return err
	}
	if view.syncBinding.check(control.syncBinding, true) != nil || view.Qualification != "transport_only" || !receiptFalse(view.Qualified) || !receiptFalse(view.After) || view.Permission != original.Permission {
		return syncRefuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
	}
	var current struct {
		ID          string `json:"id"`
		Content     string `json:"contentVersion"`
		Scope       string `json:"deleteScope"`
		Unavailable bool   `json:"contentUnavailable"`
		Deleted     bool   `json:"deleted"`
		Forwarded   bool   `json:"forwarded"`
	}
	if err = c.call("GET", "/api/messages/"+url.PathEscape(parent.Message), nil, &current); err != nil {
		return err
	}
	if current.ID != parent.Message || current.Content != parent.Content || current.Deleted || current.Forwarded || !current.Unavailable || current.Scope != "cloud_only" {
		return syncRefuse("MIGRATION_CLASSIFICATION_UNKNOWN")
	}
	return nil
}
