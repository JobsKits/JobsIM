package jobsim

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func passwordChangeTestRequest() passwordChangeRequest {
	return passwordChangeRequest{RequestID: "own-change-" + newID(), Username: "owner_one", DeviceID: newID(), RestoreEpoch: "1", CurrentPassword: "Current password example 19", NewPassword: "Replacement password example 29"}
}

func TestPasswordChangeDecodeRejectsAmbiguousOrUnboundedInput(t *testing.T) {
	q := passwordChangeTestRequest()
	raw, _ := json.Marshal(q)
	for _, body := range [][]byte{
		append(append([]byte{}, raw...), []byte(" {}")...),
		[]byte(strings.Replace(string(raw), `"username":`, `"unexpected":1,"username":`, 1)),
		[]byte(strings.Replace(string(raw), "owner_one", string([]byte{'a', 0xff, 'b'}), 1)),
		[]byte(strings.Replace(string(raw), q.CurrentPassword, strings.Repeat("a", 9000), 1)),
		[]byte(strings.Replace(string(raw), `"restoreEpoch":"1"`, `"restoreEpoch":"01"`, 1)),
	} {
		if _, err := decodePasswordChange(httptest.NewRequest("POST", "/", bytes.NewReader(body))); err == nil {
			t.Fatal("malformed credential request was accepted")
		}
	}
	q.Username, q.NewPassword = "OWNER_ONE", "Replacement cafe\u0301 password 29"
	raw, _ = json.Marshal(q)
	normalized, err := decodePasswordChange(httptest.NewRequest("POST", "/", bytes.NewReader(raw)))
	if err != nil || normalized.Username != "owner_one" || normalized.NewPassword != "Replacement café password 29" {
		t.Fatal("canonical username/password normalization failed")
	}
}

func TestPasswordChangeProofBindsOriginalIntentAndPurpose(t *testing.T) {
	a := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{7}, 32)}}
	q := passwordChangeTestRequest()
	original := a.passwordChangeDigest(q)
	withOTP := q
	withOTP.OTP = "123456"
	if !bytes.Equal(original, a.passwordChangeDigest(withOTP)) {
		t.Fatal("an expired authorization factor became the durable original intent")
	}
	for _, mutate := range []func(*passwordChangeRequest){
		func(v *passwordChangeRequest) { v.RequestID += "x" },
		func(v *passwordChangeRequest) { v.Username = "another_owner" },
		func(v *passwordChangeRequest) { v.DeviceID = newID() },
		func(v *passwordChangeRequest) { v.RestoreEpoch = "2" },
		func(v *passwordChangeRequest) { v.CurrentPassword += "x" },
		func(v *passwordChangeRequest) { v.NewPassword += "x" },
	} {
		changed := q
		mutate(&changed)
		if bytes.Equal(original, a.passwordChangeDigest(changed)) {
			t.Fatal("original proof crossed its intent or identity binding")
		}
	}
	raw, _ := json.Marshal(q)
	if bytes.Equal(original, tokenDigest(string(raw))) || bytes.Equal(original, keyedDigest(a.Config.Key, "password-reset-grant", raw)) {
		t.Fatal("stored proof was unkeyed or crossed reset-grant purpose")
	}
}

func TestPasswordChangeSealedReplacementCannotCrossContext(t *testing.T) {
	a := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{8}, 32)}}
	v := passwordChangeRecord{User: newID(), Device: newID(), Epoch: "1", Operation: "password-change:test"}
	ciphertext, err := a.seal("non-secret-test-hash", a.passwordChangeAAD(v))
	if err != nil {
		t.Fatal("replacement sealing failed")
	}
	for _, mutate := range []func(*passwordChangeRecord){
		func(x *passwordChangeRecord) { x.User = newID() },
		func(x *passwordChangeRecord) { x.Device = newID() },
		func(x *passwordChangeRecord) { x.Epoch = "2" },
		func(x *passwordChangeRecord) { x.Operation += "x" },
	} {
		other := v
		mutate(&other)
		if _, err := a.unseal(ciphertext, a.passwordChangeAAD(other)); err == nil {
			t.Fatal("replacement credential crossed its original context")
		}
	}
	a.Config.DeploymentID = newID()
	if _, err := a.unseal(ciphertext, a.passwordChangeAAD(v)); err == nil {
		t.Fatal("replacement credential crossed deployments")
	}
}

func TestPasswordChangeResultIsOutsideMutatingExportDrain(t *testing.T) {
	a := &App{Role: "identity"}
	shared, exclusive := a.personalExportOwnerFence("POST /api/auth/password-change-results")
	if shared || exclusive {
		t.Fatal("pure original-result lookup acquired mutating cleanup wrapper")
	}
	_, exclusive = a.personalExportOwnerFence("POST /api/auth/password-change")
	if !exclusive {
		t.Fatal("credential mutation lost the outer export source fence")
	}
}
