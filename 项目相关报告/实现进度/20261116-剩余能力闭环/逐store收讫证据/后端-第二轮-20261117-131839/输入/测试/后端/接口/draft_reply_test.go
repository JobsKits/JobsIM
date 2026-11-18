package jobsim

import "testing"

func TestDraftReplyAdmissionHasIndependentPurposeAndFrozenQualification(t *testing.T) {
	app := &App{Config: Config{Key: make([]byte, 32)}}
	user, membership := newID(), "7"
	message := Message{ID: newID(), ConversationID: newID(), ContentVersion: "4", Version: "20"}
	principal := forwardPrincipal{SecurityVersion: "11", RestoreEpoch: "2"}
	proof := app.replyAdmission(user, message, membership, principal)
	if proof == app.forwardAdmission(user, message, membership, principal) {
		t.Fatal("a forwarding capability was accepted as a draft-reply capability")
	}
	interaction := message
	interaction.Version = "21"
	if app.replyAdmission(user, interaction, membership, principal) != proof {
		t.Fatal("a personal interaction changed the frozen source content qualification")
	}
	for _, mutate := range []func(*Message, *forwardPrincipal, *string){
		func(m *Message, _ *forwardPrincipal, _ *string) { m.ContentVersion = "5" },
		func(m *Message, _ *forwardPrincipal, _ *string) { m.ID = newID() },
		func(_ *Message, p *forwardPrincipal, _ *string) { p.SecurityVersion = "12" },
		func(_ *Message, p *forwardPrincipal, _ *string) { p.RestoreEpoch = "3" },
		func(_ *Message, _ *forwardPrincipal, g *string) { *g = "8" },
	} {
		m, p, generation := message, principal, membership
		mutate(&m, &p, &generation)
		if app.replyAdmission(user, m, generation, p) == proof {
			t.Fatal("a later source version or security/membership/restore grant revived the original intent")
		}
	}
}
