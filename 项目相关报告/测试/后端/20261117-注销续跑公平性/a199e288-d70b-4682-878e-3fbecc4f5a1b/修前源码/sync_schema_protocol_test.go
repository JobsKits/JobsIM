package jobsim

import (
	"errors"
	"testing"
)

func TestSyncSchemaIsBoundWithoutChangingChatIdentity(t *testing.T) {
	app := &App{Config: Config{Key: make([]byte, 32), DeploymentID: newID()}}
	user := User{ID: newID(), DeviceID: newID(), RestoreEpoch: "2"}
	first := syncRegistration{Store: newID(), Stream: newID(), Schema: "1"}
	second := syncRegistration{Store: first.Store, Stream: first.Stream, Schema: "2"}
	for _, position := range []string{"0", "9007199254740993", "9223372036854775807"} {
		cursor := app.syncEncodeCursor(user, second, position, "permission")
		decoded, err := app.syncDecodeCursor(cursor, user, second)
		if err != nil || decoded.Position != position || decoded.User != user.ID || decoded.Source != "personal" || decoded.Device != user.DeviceID || decoded.Store != first.Store || decoded.Epoch != "2" {
			t.Fatal("protocol selection changed identity, physical store or exact position")
		}
		_, err = app.syncDecodeCursor(cursor, user, first)
		var rejection *apiError
		if !errors.As(err, &rejection) || rejection.Code != "SYNC_CONTEXT_CHANGED" {
			t.Fatal("a second schema boundary crossed into the first schema stream")
		}
	}
	// An old schema-one boundary has no added JSON schema field and remains valid.
	legacy := syncRegistration{Store: first.Store, Stream: first.Stream}
	if _, err := app.syncDecodeCursor(app.syncEncodeCursor(user, legacy, "7", "permission"), user, first); err != nil {
		t.Fatal("schema-one cursor compatibility was lost")
	}
}

func TestSyncSchemaOneNeverAllocatesNewPrivateKinds(t *testing.T) {
	for _, kind := range []string{"preferences", "conversation_view", "read_state", "read_observation", "draft"} {
		if syncSchemaIncludes("1", kind) || syncSchemaIncludes("", kind) || !syncSchemaIncludes("2", kind) {
			t.Fatalf("new private kind was not negotiated: %s", kind)
		}
	}
	for _, kind := range []string{"account", "devices", "profile", "contacts", "relationship", "contact_request", "conversation", "announcement", "message"} {
		if !syncSchemaIncludes("1", kind) || !syncSchemaIncludes("2", kind) {
			t.Fatal("an original covered kind disappeared")
		}
	}
	if len(syncSchemaOwners("1")) != 4 || len(syncSchemaOwners("2")) != 5 || syncSchemaOwners("2")[4] != "sync" {
		t.Fatal("schema-one source coverage or schema-two self owner changed unexpectedly")
	}
}

func TestSyncNewPrivateWireCannotTargetAnotherSource(t *testing.T) {
	scope, owner, conversation := newID(), newID(), newID()
	for _, kind := range []string{"preferences", "conversation_view", "read_state", "read_observation", "draft"} {
		ref := syncRef{Owner: syncKindOwner(kind), Kind: kind, ID: newID(), SubjectUserID: owner, Revision: "12", RevisionEpoch: "2", State: "active"}
		if kind == "preferences" {
			ref.ID = owner
		} else {
			ref.ConversationID = conversation
		}
		wire := syncWireEvent{Owner: ref.Owner, OriginEventID: newID(), ScopeID: scope, RestoreEpoch: "2", Revision: "12", Ref: ref}
		subject := "jobsim." + scope + ".2." + ref.Owner
		if syncNATSValidate(wire, subject, scope, "2") != nil || !syncValidationRef(ref.Owner, owner, ref) {
			t.Fatalf("valid private source pointer rejected: %s", kind)
		}
		wire.Ref.TargetUserID = newID()
		if syncNATSValidate(wire, subject, scope, "2") == nil {
			t.Fatal("private pointer accepted a foreign target")
		}
	}
}
