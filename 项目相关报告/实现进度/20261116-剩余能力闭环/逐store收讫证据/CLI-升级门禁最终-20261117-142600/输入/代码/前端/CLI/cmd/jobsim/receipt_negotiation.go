// Created by Jobs on 2026年11月17日，星期二.
package main

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const receiptNegotiationProtocol = "qualified-receipts-v1"

type receiptReadiness struct {
	TransportConfigured *bool    `json:"transportConfigured"`
	PublishConnected    *bool    `json:"publishConnected"`
	ConsumeConnected    *bool    `json:"consumeConnected"`
	MessageDurable      *bool    `json:"messageDurable"`
	JournalDurable      *bool    `json:"journalDurable"`
	JournalIndependent  *bool    `json:"journalIndependent"`
	EpochMatches        *bool    `json:"epochMatches"`
	Ready               *bool    `json:"ready"`
	Blockers            []string `json:"blockers"`
}

type receiptNegotiation struct {
	syncBinding
	Protocol      string           `json:"protocol"`
	Schema        string           `json:"schemaVersion"`
	Qualification string           `json:"qualification"`
	Storage       receiptStorage   `json:"storage"`
	Qualified     *bool            `json:"qualifiedAck"`
	Content       *bool            `json:"contentReceiptEnabled"`
	Object        *bool            `json:"objectReceiptEnabled"`
	After         *bool            `json:"afterReceiptEnabled"`
	AfterObject   *bool            `json:"afterObjectReceiptEnabled"`
	Readiness     receiptReadiness `json:"readiness"`
	Default       string           `json:"defaultRetentionMode"`
	Deletion      *bool            `json:"deletionEligible"`
	Target        *bool            `json:"targetRangeComplete"`
}

type receiptApprovedProfile struct {
	ID           string `json:"id"`
	Build        string `json:"buildDigest"`
	Verification string `json:"verificationDigest"`
	Contract     string `json:"storageContract"`
}

type receiptProfileList struct {
	Protocol    string                   `json:"protocol"`
	Profiles    []receiptApprovedProfile `json:"profiles"`
	Qualified   *bool                    `json:"qualifiedAck"`
	Declaration *bool                    `json:"declarationRequired"`
	LocalProbe  *bool                    `json:"localDurabilityVerificationRequired"`
}

// The common authenticated request path still owns refresh and late-response fencing.
func (c *client) receiptRequestHeaders(req *http.Request) error {
	path := req.URL.Path
	qualified := path == "/api/sync/receipt-context" || path == "/api/sync/receipt-profiles" || strings.HasPrefix(path, "/api/sync/receipt-targets/")
	policy := req.Method == "PUT" && strings.HasPrefix(path, "/api/conversations/") && strings.HasSuffix(path, "/retention")
	if !qualified && !policy {
		return nil
	}
	s, current, err := c.receiptContext()
	if err != nil {
		return err
	}
	control, err := s.receiptControl(s.db, nil, current, false)
	if err != nil {
		return err
	}
	req.Header.Set("X-JobsIM-Store-Generation", control.StoreGeneration)
	return nil
}

func receiptBool(value *bool, expected bool) bool {
	return value != nil && *value == expected
}

func receiptValidateNegotiation(v receiptNegotiation, control syncControl) error {
	if v.Protocol != receiptNegotiationProtocol || v.syncBinding.check(control.syncBinding, false) != nil || v.Schema != control.schema() || v.Default != "cloud_history" || !receiptFalse(v.Deletion) || !receiptFalse(v.Target) {
		return syncRefuse("RECEIPT_NEGOTIATION_INVALID")
	}
	no := false
	if err := receiptCheckStorage(receiptStorageView{Storage: v.Storage, After: &no, Deletion: &no}, control.syncBinding); err != nil {
		return err
	}
	if v.Storage.Qualified && !oneOf(v.Storage.Contract, "native_sqlite_full_v1", receiptFileContract) {
		return syncRefuse("RECEIPT_NEGOTIATION_INVALID")
	}
	qualification := "transport_only"
	if v.Storage.Qualified {
		qualification = "qualified_content"
	}
	if v.Storage.ObjectQualified {
		qualification = "qualified_content_and_objects"
	}
	r := v.Readiness
	flags := []*bool{r.TransportConfigured, r.PublishConnected, r.ConsumeConnected, r.MessageDurable, r.JournalDurable, r.JournalIndependent, r.EpochMatches}
	ready := true
	for _, flag := range flags {
		if flag == nil {
			return syncRefuse("RECEIPT_NEGOTIATION_INVALID")
		}
		ready = ready && *flag
	}
	if v.Qualification != qualification || !receiptBool(v.Qualified, v.Storage.Qualified) || !receiptBool(v.Content, v.Storage.Qualified) || !receiptBool(v.Object, v.Storage.ObjectQualified) || !receiptBool(r.Ready, ready) || r.Blockers == nil || ready && len(r.Blockers) != 0 || !receiptBool(v.After, v.Storage.Qualified && ready) || !receiptBool(v.AfterObject, v.Storage.ObjectQualified && ready) {
		return syncRefuse("RECEIPT_NEGOTIATION_INVALID")
	}
	return nil
}

func (c *client) getReceiptNegotiation(control syncControl) (receiptNegotiation, error) {
	var v receiptNegotiation
	s, current, err := c.receiptContext()
	if err != nil {
		return v, err
	}
	if _, err = s.receiptControl(s.db, &control, current, false); err != nil {
		return v, err
	}
	if err = c.call("GET", syncSchemaPath("/api/sync/receipt-context", control.schema()), nil, &v); err != nil {
		return v, err
	}
	if _, err = s.receiptControl(s.db, &control, current, false); err != nil {
		return v, err
	}
	return v, receiptValidateNegotiation(v, control)
}

func (c *client) receiptProfiles(control syncControl, build string) (receiptProfileList, error) {
	var v receiptProfileList
	if !syncDigestPattern.MatchString(build) {
		return v, syncRefuse("RECEIPT_BUILD_MISMATCH")
	}
	if err := c.receiptNegotiationCurrent(control); err != nil {
		return v, err
	}
	path := syncSchemaPath("/api/sync/receipt-profiles?buildDigest="+url.QueryEscape(build), control.schema())
	err := c.call("GET", path, nil, &v)
	if err != nil {
		return v, err
	}
	if err = c.receiptNegotiationCurrent(control); err != nil {
		return v, err
	}
	if v.Protocol != receiptNegotiationProtocol || !receiptFalse(v.Qualified) || !receiptBool(v.Declaration, true) || !receiptBool(v.LocalProbe, true) || v.Profiles == nil || len(v.Profiles) > 64 {
		return v, syncRefuse("RECEIPT_PROFILES_INVALID")
	}
	seen := map[string]bool{}
	for _, p := range v.Profiles {
		if !receiptProfileID(p.ID) || p.Build != build || !syncDigestPattern.MatchString(p.Verification) || !oneOf(p.Contract, "native_sqlite_full_v1", receiptFileContract) || seen[p.ID] {
			return v, syncRefuse("RECEIPT_PROFILES_INVALID")
		}
		seen[p.ID] = true
	}
	return v, nil
}

func (c *client) receiptNegotiationCurrent(control syncControl) error {
	s, current, err := c.receiptContext()
	if err != nil {
		return err
	}
	_, err = s.receiptControl(s.db, &control, current, false)
	return err
}

func (c *client) receiptPrepareDeclaration(control syncControl, profile, build, verification string, current func() bool) error {
	profiles, err := c.receiptProfiles(control, build)
	if err != nil {
		return err
	}
	for _, p := range profiles.Profiles {
		if p.ID == profile && p.Build == build && p.Verification == verification {
			return c.store.receiptNativeProbe(p.Contract, control, current)
		}
	}
	return syncRefuse("RECEIPT_PROFILE_NOT_APPROVED")
}

// A successful probe is local evidence only, never deployment approval or a message ACK.
func (s *localStore) receiptNativeProbe(contract string, control syncControl, current func() bool) error {
	if !oneOf(contract, "native_sqlite_full_v1", receiptFileContract) {
		return syncRefuse("RECEIPT_STORAGE_CONTRACT_REQUIRED")
	}
	if _, err := s.receiptControl(s.db, &control, current, true); err != nil {
		return err
	}
	if err := receiptDurability(s.db); err != nil {
		return err
	}
	probe := make([]byte, 128)
	if _, err := rand.Read(probe); err != nil {
		return err
	}
	digest := fingerprint(string(probe))
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.receiptControl(tx, &control, current, true); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO receipt_native_probe(id,epoch,store_generation,payload_hash) VALUES(1,?,?,?) ON CONFLICT(id) DO UPDATE SET epoch=excluded.epoch,store_generation=excluded.store_generation,payload_hash=excluded.payload_hash", s.epoch, control.StoreGeneration, digest); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	var actual string
	if err = s.db.QueryRow("SELECT payload_hash FROM receipt_native_probe WHERE id=1 AND epoch=? AND store_generation=?", s.epoch, control.StoreGeneration).Scan(&actual); err != nil || actual != digest {
		return syncRefuse("RECEIPT_STORAGE_PROBE_FAILED")
	}
	if contract == receiptFileContract {
		root := filepath.Dir(s.path)
		if err := receiptDirectory(root, false); err != nil {
			return err
		}
		f, err := os.CreateTemp(root, ".receipt-native-probe-")
		if err != nil {
			return err
		}
		name := f.Name()
		defer os.Remove(name)
		defer f.Close()
		data := make([]byte, 128)
		if _, err = rand.Read(data); err != nil {
			return err
		}
		if _, err = f.Write(data); err != nil {
			return err
		}
		if err = f.Sync(); err != nil {
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = receiptSyncDirectory(root); err != nil {
			return err
		}
		got, err := os.ReadFile(name)
		if err != nil || fingerprint(string(got)) != fingerprint(string(data)) {
			return syncRefuse("RECEIPT_STORAGE_PROBE_FAILED")
		}
		if err = os.Remove(name); err != nil {
			return err
		}
		if err = receiptSyncDirectory(root); err != nil {
			return err
		}
	}
	_, err = s.receiptControl(s.db, &control, current, true)
	return err
}

func (c *client) receiptNegotiationCommand(cmd, args string, control syncControl) error {
	switch cmd {
	case "receipt-context":
		v, err := c.getReceiptNegotiation(control)
		if err != nil {
			return err
		}
		return printJSON(v)
	case "receipt-profiles":
		build, err := receiptExecutableDigest()
		if err != nil {
			return err
		}
		v, err := c.receiptProfiles(control, build)
		if err != nil {
			return err
		}
		return printJSON(v)
	case "receipt-target":
		id := strings.TrimSpace(args)
		if !syncUUIDPattern.MatchString(id) {
			return syncRefuse("INVALID_MESSAGE_ID")
		}
		var v receiptTargetView
		if err := c.call("GET", syncSchemaPath("/api/sync/receipt-targets/"+url.PathEscape(id), control.schema()), nil, &v); err != nil {
			return err
		}
		if err := c.receiptNegotiationCurrent(control); err != nil {
			return err
		}
		if err := receiptValidateTarget(v, id, control.RestoreEpoch); err != nil {
			return err
		}
		return printJSON(v)
	}
	return fmt.Errorf("unknown receipt negotiation command")
}

type receiptTargetView struct {
	Protocol      string  `json:"protocol"`
	Message       string  `json:"messageId"`
	Content       string  `json:"contentVersion"`
	Epoch         string  `json:"restoreEpoch"`
	Qualification string  `json:"qualificationId"`
	Generation    string  `json:"deliveryGeneration"`
	Digest        string  `json:"manifestDigest"`
	Scope         string  `json:"receiptScope"`
	Original      *bool   `json:"originalTarget"`
	ContentFor    *bool   `json:"contentTarget"`
	ObjectFor     *bool   `json:"objectTarget"`
	ContentACK    *bool   `json:"contentRecorded"`
	ObjectACK     *bool   `json:"objectRecorded"`
	Complete      *string `json:"contentCompleteAt"`
	ObjectDone    *string `json:"objectCompleteAt"`
	Current       *bool   `json:"current"`
	MessageNow    *bool   `json:"messageCurrent"`
	Deletion      *bool   `json:"deletionEligible"`
}

func receiptValidateTarget(v receiptTargetView, id, epoch string) error {
	if v.Protocol != receiptNegotiationProtocol || v.Message != id || v.Epoch != epoch || !oneOf(v.Scope, "all_target_devices", "per_user_any_device") || !receiptFalse(v.Deletion) || v.Generation == "" || receiptDeliveryPair(v.Generation, v.Digest) != nil || v.Original == nil || v.ContentFor == nil || v.ObjectFor == nil || v.ContentACK == nil || v.ObjectACK == nil || v.Current == nil || (*v.ContentFor || *v.ObjectFor) && (!*v.Original || !*v.Current) || *v.ObjectFor && !*v.ContentFor || *v.ObjectACK && !*v.ContentACK {
		return syncRefuse("RECEIPT_TARGET_INVALID")
	}
	if n, err := syncCounter(v.Content); err != nil || n == 0 {
		return syncRefuse("RECEIPT_TARGET_INVALID")
	}
	return nil
}
