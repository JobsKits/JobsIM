package jobsim

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelfViewIntentRejectsAmbiguousFields(t *testing.T) {
	base := `"requestId":"original-view-intent","revisionEpoch":"2","expectedVersion":"9007199254740993",`
	for _, changes := range []string{
		`{"hidden":null,"archived":true}`,
		`{"manualUnread":null,"archived":true}`,
		`{"archived":null,"hidden":false}`,
		`{"hidden":false,"publicReadReceipts":true}`,
		`{"pinnedRank":1.5}`,
		`{"pinnedRank":-1}`,
		`{"mutedUntil":"2026-11-16T10:00:00+08:00"}`,
		`{"manualUnread":true}`,
		`{"manualUnread":false,"expectedManualUnreadGeneration":"01"}`,
		`{"hidden":false,"expectedManualUnreadGeneration":"0"}`,
		`{}`,
	} {
		t.Run(changes, func(t *testing.T) {
			request := httptest.NewRequest("PATCH", "/api/conversations/id/view", strings.NewReader(`{`+base+`"changes":`+changes+`}`))
			var patch selfViewPatch
			err := decodeSelfViewPatch(request, &patch)
			if err == nil {
				err = validateSelfViewPatch(patch)
			}
			if err == nil {
				t.Fatal("ambiguous, unsupported or noncanonical view mutation accepted")
			}
		})
	}
	for _, suffix := range []string{` {"changes":{"hidden":true}}`, ` garbage`} {
		request := httptest.NewRequest("PATCH", "/api/conversations/id/view", strings.NewReader(`{`+base+`"changes":{"hidden":true}}`+suffix))
		var patch selfViewPatch
		if decodeSelfViewPatch(request, &patch) == nil {
			t.Fatal("trailing second intent accepted")
		}
	}
}

func TestSelfViewIntentKeepsExactCountersAndExplicitNulls(t *testing.T) {
	request := httptest.NewRequest("PATCH", "/api/conversations/id/view", strings.NewReader(`{"requestId":"original-view-intent","revisionEpoch":"2","expectedVersion":"9007199254740993","changes":{"pinnedRank":null,"mutedUntil":null,"hidden":false,"manualUnread":true,"expectedManualUnreadGeneration":"9223372036854775806"}}`))
	var patch selfViewPatch
	if err := decodeSelfViewPatch(request, &patch); err != nil {
		t.Fatal(err)
	}
	if err := validateSelfViewPatch(patch); err != nil {
		t.Fatal(err)
	}
	if patch.ExpectedVersion != "9007199254740993" || patch.Changes.ExpectedManualUnreadGeneration != "9223372036854775806" || string(patch.Changes.PinnedRank) != "null" || patch.Changes.Hidden == nil || *patch.Changes.Hidden {
		t.Fatal("raw null, exact counter or explicit false intent was lost")
	}
}

func TestPrivateStateGatewayUsesSharedResponseFence(t *testing.T) {
	for _, path := range []string{"/api/conversations/id/read", "/api/conversations/id/read-results", "/api/conversations/id/view", "/api/conversations/id/view-results", "/api/conversations/id/draft", "/api/conversations/id/draft-results", "/api/me/preferences", "/api/me/preferences-results", "/api/me/preferences/operations/original-request"} {
		request := httptest.NewRequest("POST", path, nil)
		if gatewayProjectionMode(request) != "shared" {
			t.Fatalf("private state response has no shared instance fence: %s", path)
		}
	}
	for _, path := range []string{"/api/conversations/id/unknown", "/api/notconversations/id/view", "/api/conversations/id/view/extra"} {
		if selfStatePublicPath(path) {
			t.Fatalf("unrelated route was captured as personal state: %s", path)
		}
	}
}
