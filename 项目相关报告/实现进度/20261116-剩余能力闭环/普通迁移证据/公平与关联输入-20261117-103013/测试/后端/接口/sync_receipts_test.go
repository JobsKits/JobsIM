package jobsim

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func receiptTestProfile() ReceiptProfile {
	return ReceiptProfile{ID: "android-full-test", BuildDigest: strings.Repeat("1", 64), VerificationDigest: strings.Repeat("2", 64), StorageContract: "native_sqlite_full_v1"}
}

func receiptTestError(t *testing.T, err error, code string) {
	t.Helper()
	var failure *apiError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatal("receipt did not produce its expected bounded rejection")
	}
}

func TestReceiptPolicyRequiresExactNativeBuildAndEvidence(t *testing.T) {
	profile := receiptTestProfile()
	app := &App{Config: Config{ReceiptProfiles: []ReceiptProfile{profile}}}
	if validateReceiptProfiles(app.Config.ReceiptProfiles) != nil {
		t.Fatal("exact native profile rejected")
	}
	if _, allowed := app.receiptProfile(profile.ID, profile.BuildDigest, profile.VerificationDigest); !allowed {
		t.Fatal("approved exact tuple not found")
	}
	for _, changed := range []ReceiptProfile{
		{ID: profile.ID, BuildDigest: strings.Repeat("3", 64), VerificationDigest: profile.VerificationDigest},
		{ID: profile.ID, BuildDigest: profile.BuildDigest, VerificationDigest: strings.Repeat("3", 64)},
		{ID: "browser-cache", BuildDigest: profile.BuildDigest, VerificationDigest: profile.VerificationDigest},
	} {
		if _, allowed := app.receiptProfile(changed.ID, changed.BuildDigest, changed.VerificationDigest); allowed {
			t.Fatal("unapproved build, evidence or browser profile inherited qualification")
		}
	}
	for _, contract := range []string{"indexeddb", "sqlite_default", "native_sqlite_full_v2", ""} {
		changed := profile
		changed.StorageContract = contract
		if validateReceiptProfiles([]ReceiptProfile{changed}) == nil {
			t.Fatal("unsupported durability contract was approved")
		}
	}
	if validateReceiptProfiles([]ReceiptProfile{profile, profile}) == nil {
		t.Fatal("duplicate profile accepted")
	}
	app.Config.ReceiptProfiles = nil
	if _, allowed := app.receiptProfile(profile.ID, profile.BuildDigest, profile.VerificationDigest); allowed {
		t.Fatal("removed deployment policy remained qualified")
	}
}

func TestReceiptPolicyFileIsBoundedAndStrict(t *testing.T) {
	profile := receiptTestProfile()
	raw, _ := json.Marshal(map[string]any{"profiles": []ReceiptProfile{profile}})
	file := filepath.Join(t.TempDir(), "receipt-policy.json")
	if os.WriteFile(file, raw, 0600) != nil {
		t.Fatal("private policy fixture unavailable")
	}
	loaded, err := LoadReceiptProfiles(file)
	if err != nil || !reflect.DeepEqual(loaded, []ReceiptProfile{profile}) {
		t.Fatal("valid external policy failed")
	}
	for _, invalid := range []string{string(raw) + `{}`, `{"profiles":[],"enableAfterReceipt":true}`, strings.Repeat(" ", (1<<20)+1)} {
		if os.WriteFile(file, []byte(invalid), 0600) != nil {
			t.Fatal("policy fixture write failed")
		}
		if _, err = LoadReceiptProfiles(file); err == nil {
			t.Fatal("ambiguous, unknown or unbounded policy accepted")
		}
	}
	if value, err := LoadReceiptProfiles(""); err != nil || value != nil {
		t.Fatal("default deployment must keep transport-only registration")
	}
}

func TestReceiptProofBindsExactContentStoreAndRestoreEpoch(t *testing.T) {
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x49}, 32)}}
	now := time.Now().UTC().Truncate(time.Second)
	proof := receiptProof{Scope: app.Config.DeploymentID, User: newID(), Device: newID(), Store: newID(), Epoch: "9007199254740993", Qualification: newID(), Message: newID(), Conversation: newID(), Content: "9223372036854775807", BodyDigest: strings.Repeat("a", 64), Permission: strings.Repeat("b", 64), Expires: now.Add(time.Hour).Unix()}
	token := app.encodeReceiptProof(proof)
	decoded, err := app.decodeReceiptProof(token, now)
	if err != nil || decoded != proof {
		t.Fatal("original exact integers or identity tuple changed")
	}
	parts := strings.Split(token, ".")
	parts[1] = parts[1][:len(parts[1])-1] + "!"
	_, err = app.decodeReceiptProof(strings.Join(parts, "."), now)
	receiptTestError(t, err, "INVALID_RECEIPT_PROOF")
	other := *app
	other.Config.DeploymentID = newID()
	_, err = other.decodeReceiptProof(token, now)
	receiptTestError(t, err, "INVALID_RECEIPT_PROOF")
	_, err = app.decodeReceiptProof(token, now.Add(time.Hour))
	receiptTestError(t, err, "RECEIPT_PROOF_EXPIRED")
	proof.Content = "01"
	_, err = app.decodeReceiptProof(app.encodeReceiptProof(proof), now)
	receiptTestError(t, err, "INVALID_RECEIPT_PROOF")
}

func TestReceiptRequestsRejectTailUnknownFieldsAndNumericEpoch(t *testing.T) {
	for _, raw := range []string{`{"requestId":"12345678"}{}`, `{"userId":"foreign"}`, `{"revisionEpoch":1}`, strings.Repeat("x", 33<<10)} {
		request := httptest.NewRequest("POST", "/api/sync/receipts", strings.NewReader(raw))
		var value contentReceiptRequest
		receiptTestError(t, decodeReceiptRequest(request, &value), "INVALID_RECEIPT_REQUEST")
	}
	one, _ := receiptJSONValue([]byte(`{"value":9007199254740992}`))
	two, _ := receiptJSONValue([]byte(`{"value":9007199254740993}`))
	if reflect.DeepEqual(one, two) {
		t.Fatal("envelope comparison rounded distinct large integers")
	}
	for _, value := range []string{"       x", "1234567", "12345678\n", "1234\x005678"} {
		if validReceiptNonce(value) {
			t.Fatal("noncanonical receipt nonce accepted")
		}
	}
}
