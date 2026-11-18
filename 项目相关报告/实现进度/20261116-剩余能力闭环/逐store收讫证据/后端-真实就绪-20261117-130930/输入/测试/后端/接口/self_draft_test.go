package jobsim

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelfDraftOriginalBodyAndScopeCannotBeSubstituted(t *testing.T) {
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x62}, 32)}}
	u, conversation := User{ID: newID(), RestoreEpoch: "1"}, newID()
	patch := selfDraftPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "0", DraftAdmission: "da1.original", OwnedText: "independent own input"}
	if err := validateSelfDraftPatch("PUT", patch); err != nil {
		t.Fatal("owned text intent fixture rejected")
	}
	original := app.selfDraftDigest(u, conversation, "PUT", patch)
	changed := patch
	changed.OwnedText = "another independent input"
	if bytes.Equal(original, app.selfDraftDigest(u, conversation, "PUT", changed)) || bytes.Equal(original, app.selfDraftDigest(u, newID(), "PUT", patch)) || bytes.Equal(original, app.selfDraftDigest(User{ID: newID()}, conversation, "PUT", patch)) || bytes.Equal(original, app.selfDraftDigest(u, conversation, "DELETE", patch)) {
		t.Fatal("original draft body, method, principal or conversation was omitted from the proof digest")
	}
	changed = patch
	changed.Reply = &selfDraftReply{MessageID: newID(), SourceVersion: "3", ReplyAdmission: "dr1.original"}
	firstReply := app.selfDraftDigest(u, conversation, "PUT", changed)
	changed.Reply.ReplyAdmission = "dr1.later-qualification"
	if bytes.Equal(firstReply, app.selfDraftDigest(u, conversation, "PUT", changed)) {
		t.Fatal("a later source grant silently repaired an already frozen draft body")
	}
}

func TestSelfDraftStrictJSONAndTextBounds(t *testing.T) {
	for _, body := range []string{
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":"0","draftAdmission":"da1.original","ownedText":null}`,
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":"0","draftAdmission":"da1.original"}`,
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":0,"draftAdmission":"da1.original","ownedText":"text","reply":null}`,
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":"0","draftAdmission":"da1.original","ownedText":"text","sourceContextId":"foreign"}`,
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":"0","draftAdmission":"da1.original","ownedText":"text"} {"second":true}`,
		`{"requestId":"original-nonce","revisionEpoch":"1","expectedVersion":"0","draftAdmission":"da1.original","ownedText":"text","reply":{"messageId":"invalid","sourceVersion":"1","replyAdmission":"dr1.original"}}`,
	} {
		request := httptest.NewRequest("PUT", "/api/conversations/unused/draft", strings.NewReader(body))
		if _, err := decodeSelfDraftPatch(request, "PUT"); err == nil {
			t.Fatal("ambiguous or foreign draft write body was accepted")
		}
	}
	patch := selfDraftPatch{RequestID: newID(), RevisionEpoch: "1", ExpectedVersion: "9007199254740993", DraftAdmission: "da1.original", OwnedText: strings.Repeat("界", 5461)}
	if err := validateSelfDraftPatch("PUT", patch); err != nil {
		t.Fatal("exact large CAS counter or bounded multibyte text rejected")
	}
	patch.OwnedText += "界"
	if err := validateSelfDraftPatch("PUT", patch); err == nil {
		t.Fatal("byte-bound draft text limit was treated as character count")
	}
	if err := validateSelfDraftPatch("DELETE", patch); err == nil {
		t.Fatal("clear intent carried an implicit replacement text")
	}
}

func TestSelfDraftJournalNeverCarriesOwnedTextOrReplyProof(t *testing.T) {
	fields := draftJournalKinds()["sync.draft_floor"]
	for _, forbidden := range []string{"ownedText", "replyAdmission", "draftAdmission", "quote", "text", "ciphertext"} {
		for _, field := range fields {
			if field == forbidden {
				t.Fatal("draft safety journal became another body or grant store")
			}
		}
	}
	for field, value := range map[string]string{"epoch": "0", "contentDigest": "not-a-digest", "action": "publish-old-text", "replyState": "trusted-later", "operation": "not-a-uuid"} {
		if handled, err := validateDraftJournalField(field, value); !handled || err == nil {
			t.Fatal("malformed typed draft denial entered the journal")
		}
	}
	app := &App{Config: Config{DeploymentID: newID(), Key: bytes.Repeat([]byte{0x63}, 32)}}
	identity := app.selfDraftIdentity(User{ID: newID(), RestoreEpoch: "1"}, newID())
	identity.Version, identity.Operation, identity.Action, identity.ReplyState = "1", newID(), "set", "available"
	identity.ContentDigest, _ = app.draftOwnedDigest(identity, "independent owned input")
	raw, _ := json.Marshal(identity)
	if bytes.Contains(raw, []byte("independent owned input")) || bytes.Contains(raw, []byte("Admission")) {
		t.Fatal("typed floor retained an owned body or source grant")
	}
}
