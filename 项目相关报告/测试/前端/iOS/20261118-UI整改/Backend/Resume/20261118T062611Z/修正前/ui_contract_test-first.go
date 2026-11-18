package jobsim

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uiRegressionCall(a *App, method, path, token string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	if body == nil {
		raw = nil
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func uiRegressionPeer(t *testing.T, f *deletionPGFixture) (User, string) {
	t.Helper()
	a := f.nodes["identity"]
	u := User{ID: newID(), Username: "ui_" + strings.ReplaceAll(newID(), "-", "")[:20], DisplayName: "UI isolated peer"}
	deletionExec(t, f.ctx, a, `INSERT INTO auth.users(id,username,display_name,password_hash) VALUES($1,$2,$3,'fixture-not-a-login-hash')`, u.ID, u.Username, u.DisplayName)
	deletionExec(t, f.ctx, a, `INSERT INTO directory.profiles(user_id,display_name) VALUES($1,$2),($3,$4) ON CONFLICT DO NOTHING`, u.ID, u.DisplayName, f.user.ID, f.user.DisplayName)
	tx, err := a.DB.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.issueSession(f.ctx, tx, u, "ui-isolated-native", 1, 1, 1, false)
	if err != nil {
		tx.Rollback(f.ctx)
		t.Fatal("fixture family unavailable")
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	u.DeviceID, u.RestoreEpoch = d.Result.DeviceID, d.Result.RestoreEpoch
	return u, d.Result.Token
}

func uiRegressionFriend(t *testing.T, f *deletionPGFixture, peer User, token string) string {
	t.Helper()
	q := map[string]string{"userId": peer.ID, "requestId": newID()}
	v := deletionBody(t, uiRegressionCall(f.nodes["directory"], "POST", "/api/contacts/requests", f.token, q), 200)
	id := v["id"].(string)
	deletionBody(t, uiRegressionCall(f.nodes["directory"], "POST", "/api/contacts/requests/"+id+"/accept", token, map[string]any{}), 200)
	return id
}

func uiRegressionDirect(t *testing.T, f *deletionPGFixture, peer User) map[string]any {
	t.Helper()
	return deletionBody(t, uiRegressionCall(f.nodes["conversation"], "POST", "/api/conversations", f.token, map[string]any{"kind": "direct", "memberIds": []string{peer.ID}}), 200)
}

func uiRegressionConversation(t *testing.T, f *deletionPGFixture, user, token, id string) Conversation {
	t.Helper()
	v, err := f.nodes["conversation"].conversationByID(httptest.NewRequest("GET", "/api/conversations", nil), id, user)
	if err != nil {
		t.Fatal("current conversation unavailable")
	}
	return v
}

func uiRegressionSend(t *testing.T, f *deletionPGFixture, user, token, id, text string) map[string]any {
	t.Helper()
	c := uiRegressionConversation(t, f, user, token, id)
	q := sendPayload{ClientMsgID: newID(), Type: "text", Text: text, RestoreEpoch: "1", SendAdmission: c.SendAdmission}
	return deletionBody(t, uiRegressionCall(f.nodes["message"], "POST", "/api/conversations/"+id+"/messages", token, q), 200)
}

func TestUIReportSelectionAndUploadPurpose(t *testing.T) {
	id := newID()
	report := reportCreateRequest{TargetType: "user", TargetID: id, Reason: "original reason", RequestID: newID()}
	legacyReport, _ := json.Marshal(struct {
		TargetType string `json:"targetType"`
		TargetID   string `json:"targetId"`
		Reason     string `json:"reason"`
		RequestID  string `json:"requestId"`
	}{report.TargetType, report.TargetID, report.Reason, report.RequestID})
	currentReport, _ := json.Marshal(report)
	if !bytes.Equal(legacyReport, currentReport) {
		t.Fatal("existing four-field report digest changed")
	}
	if !validReportSelection([]string{id}, reportFileLimit) || validReportSelection([]string{id, id}, reportFileLimit) || validReportSelection([]string{"foreign"}, reportFileLimit) {
		t.Fatal("selection identity bounds failed")
	}
	q := mediaUploadRequest{RequestID: newID(), UserID: newID(), DeviceID: newID(), FamilyID: newID(), Epoch: "1", Name: "image.png", Size: "1", Digest: strings.Repeat("a", 64)}
	p := mediaUploadPrincipal{Security: "1", SourceGeneration: "1"}
	a := &App{Config: Config{Key: bytes.Repeat([]byte{5}, 32), DeploymentID: newID()}}
	legacy, _ := json.Marshal(struct {
		Request  mediaUploadRequest
		Security string
		Source   string
	}{q, p.Security, p.SourceGeneration})
	chat := a.mediaUploadBinding(q, p)
	if !bytes.Equal(chat, keyedDigest(a.Config.Key, "stable-media-upload:"+a.Config.DeploymentID, legacy)) {
		t.Fatal("existing chat upload digest changed")
	}
	q.purpose = "report_evidence"
	if bytes.Equal(chat, a.mediaUploadBinding(q, p)) {
		t.Fatal("report upload borrowed chat responsibility")
	}
	if mediaUploadPublic(mediaUploadRecord{Purpose: "report_evidence", ObjectID: id, State: "committed"}).URL != "/api/reports/evidence/"+id {
		t.Fatal("report upload escaped its dedicated route")
	}
	for _, path := range []string{"/api/reports/evidence", "/api/reports/evidence/" + id, "/api/admin/reports/" + id + "/messages/" + newID() + "/attachment"} {
		if !reportMediaPath(path) {
			t.Fatal("report media routing missing")
		}
	}
	if reportMediaPath("/api/admin/reports/" + id) {
		t.Fatal("metadata detail routed to media")
	}
}

func TestUIOptionalRetractionSnapshot(t *testing.T) {
	for _, body := range []string{"", "{}", `{"requestId":"original-request","expectedVersion":"9007199254740993"}`} {
		if _, err := decodeRetractionRequest(httptest.NewRequest("DELETE", "/api/messages/id", strings.NewReader(body))); err != nil {
			t.Fatal("valid optional snapshot rejected")
		}
	}
	for _, body := range []string{`{"expectedVersion":"1"}`, `{"requestId":"original-request","expectedVersion":"01"}`, `{"requestId":"original-request","expectedVersion":"0"}`, `{"requestId":"original-request","expectedVersion":1}`, `{"unknown":true}`, `{} {}`} {
		if _, err := decodeRetractionRequest(httptest.NewRequest("DELETE", "/api/messages/id", strings.NewReader(body))); err == nil {
			t.Fatal("malformed original snapshot accepted")
		}
	}
}

func TestUIContactDeletionNewRoundsAndPendingPG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	accepted := uiRegressionFriend(t, f, peer, token)
	d := f.nodes["directory"]
	deletionBody(t, uiRegressionCall(d, "DELETE", "/api/contacts/"+peer.ID, f.token, nil), 200)
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM directory.contacts WHERE state='friend'`) != 0 {
		t.Fatal("deletion left a one-sided friendship")
	}
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests/"+accepted+"/accept", token, map[string]any{}), 409)
	q := map[string]string{"userId": peer.ID, "requestId": newID()}
	first := deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, q), 200)
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, q), 200)
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, map[string]string{"userId": peer.ID, "requestId": newID()}), 409)
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests/"+first["id"].(string)+"/reject", token, map[string]any{}), 200)
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, q), 409)
	uiRegressionFriend(t, f, peer, token)
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM directory.contacts WHERE state='friend'`) != 2 {
		t.Fatal("new explicit round still required permanent reopening")
	}
}

func TestUIBlockKeepsFriendGroupAndRejectsOldDirectGrantPG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, peer, token)
	c := uiRegressionDirect(t, f, peer)
	id := c["id"].(string)
	old := c["sendAdmission"].(string)
	m := uiRegressionSend(t, f, f.user.ID, f.token, id, "legal committed history")
	group := deletionBody(t, uiRegressionCall(f.nodes["conversation"], "POST", "/api/conversations", f.token, map[string]any{"kind": "group", "title": "isolated shared group", "memberIds": []string{peer.ID}, "requestId": newID()}), 200)
	d := f.nodes["directory"]
	deletionBody(t, uiRegressionCall(d, "PUT", "/api/blocks/"+peer.ID, f.token, map[string]any{}), 200)
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM directory.contacts WHERE state='friend'`) != 2 {
		t.Fatal("block deleted friendship")
	}
	blocked := uiRegressionDirect(t, f, peer)
	if blocked["id"] != id || blocked["sendAdmission"] != "" {
		t.Fatal("blocked friend could not open settings or received send admission")
	}
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM conversation.members WHERE conversation_id=$1 AND state='active'`, group["id"]) != 2 {
		t.Fatal("block removed shared group qualification")
	}
	uiRegressionSend(t, f, peer.ID, token, group["id"].(string), "shared group remains independent")
	private := deletionBody(t, mediaUploadPGCall(t, f.nodes["media"], f.token, nil, "owned.txt", []byte("owned pending attachment")), 200)
	attachment := sendPayload{ClientMsgID: newID(), Type: "file", AttachmentID: private["id"].(string), RestoreEpoch: "1", SendAdmission: old}
	deletionBody(t, uiRegressionCall(f.nodes["message"], "POST", "/api/conversations/"+id+"/messages", f.token, attachment), 409)
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM media.objects WHERE id=$1 AND message_id IS NULL`, private["id"]) != 1 {
		t.Fatal("blocked direct bound a new attachment before admission")
	}
	for _, actor := range []struct{ user, token string }{{f.user.ID, f.token}, {peer.ID, token}} {
		q := sendPayload{ClientMsgID: newID(), Type: "text", Text: "must not commit", RestoreEpoch: "1", SendAdmission: old}
		deletionBody(t, uiRegressionCall(f.nodes["message"], "POST", "/api/conversations/"+id+"/messages", actor.token, q), 409)
	}
	deletionBody(t, uiRegressionCall(d, "DELETE", "/api/blocks/"+peer.ID, f.token, nil), 200)
	fresh := uiRegressionConversation(t, f, f.user.ID, f.token, id)
	if fresh.SendAdmission == "" || fresh.SendAdmission == old {
		t.Fatal("unblock revived old admission or lost existing friendship")
	}
	deletionBody(t, uiRegressionCall(f.nodes["message"], "POST", "/api/conversations/"+id+"/messages", f.token, sendPayload{ClientMsgID: newID(), Type: "text", Text: "stale queue", RestoreEpoch: "1", SendAdmission: old}), 409)
	uiRegressionSend(t, f, f.user.ID, f.token, id, "new explicit allowed message")
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM message.messages WHERE id=$1 AND NOT deleted AND text='legal committed history'`, m["id"]) != 1 {
		t.Fatal("block removed legal committed history")
	}
	other, otherToken := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, other, otherToken)
	deletionBody(t, uiRegressionCall(d, "PUT", "/api/blocks/"+other.ID, f.token, map[string]any{}), 200)
	if uiRegressionDirect(t, f, other)["sendAdmission"] != "" {
		t.Fatal("first blocked direct settings granted sends")
	}
}

func TestUIContactRateResetsWithoutPermanentGatePG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	d := f.nodes["directory"]
	for round := 0; round < 5; round++ {
		q := map[string]string{"userId": peer.ID, "requestId": newID()}
		v := deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, q), 200)
		if round == 0 {
			for duplicate := 0; duplicate < 8; duplicate++ {
				deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, map[string]string{"userId": peer.ID, "requestId": newID()}), 409)
			}
		}
		deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests/"+v["id"].(string)+"/reject", token, map[string]any{}), 200)
	}
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, map[string]string{"userId": peer.ID, "requestId": newID()}), 429)
	// Move only this isolated fixture's rate window into the past; no wall-clock or service setting changes.
	deletionExec(t, f.ctx, d, `UPDATE directory.contact_rate_windows SET window_start=window_start-interval '2 minutes'`)
	deletionBody(t, uiRegressionCall(d, "POST", "/api/contacts/requests", f.token, map[string]string{"userId": peer.ID, "requestId": newID()}), 200)
	if pcCount(t, f.ctx, d, `SELECT count(*) FROM directory.contact_requests WHERE state='pending'`) != 1 {
		t.Fatal("new round accumulated duplicate pending responsibilities")
	}
}

func uiRegressionEvidenceUpload(t *testing.T, f *deletionPGFixture, data []byte, q mediaUploadRequest) *httptest.ResponseRecorder {
	t.Helper()
	var raw bytes.Buffer
	form := multipart.NewWriter(&raw)
	for k, v := range map[string]string{"requestId": q.RequestID, "userId": q.UserID, "deviceId": q.DeviceID, "familyId": q.FamilyID, "restoreEpoch": q.Epoch, "name": q.Name, "size": q.Size, "sha256": q.Digest} {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := form.CreateFormFile("file", q.Name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/api/reports/evidence", bytes.NewReader(raw.Bytes()))
	r.Header.Set("Content-Type", form.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+f.token)
	w := httptest.NewRecorder()
	f.nodes["media"].Handler().ServeHTTP(w, r)
	return w
}

func TestUIReportEvidenceOwnershipSelectedRefsAndRevocationPG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, peer, token)
	admin, adminToken := uiRegressionPeer(t, f)
	deletionExec(t, f.ctx, f.nodes["identity"], `UPDATE auth.users SET admin=true WHERE id=$1`, admin.ID)
	c := uiRegressionDirect(t, f, peer)
	message := uiRegressionSend(t, f, peer.ID, token, c["id"].(string), "selected current source")
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	q, _ := mediaUploadPGRequest(t, f, imageBytes.Bytes())
	q.Name = "chosen.png"
	file := deletionBody(t, uiRegressionEvidenceUpload(t, f, imageBytes.Bytes(), q), 200)
	object := file["id"].(string)
	if file["url"] != "/api/reports/evidence/"+object {
		t.Fatal("evidence upload used chat delivery")
	}
	deletionBody(t, uiRegressionCall(f.nodes["media"], "GET", "/api/media/"+object, f.token, nil), 404)
	deletionBody(t, uiRegressionCall(f.nodes["media"], "GET", "/api/reports/evidence/"+object, token, nil), 404)
	deletionBody(t, uiRegressionCall(f.nodes["media"], "POST", "/api/media/upload-results", f.token, q), 409)
	deletionBody(t, uiRegressionCall(f.nodes["media"], "POST", "/api/reports/evidence/upload-results", f.token, q), 200)
	bad := reportCreateRequest{TargetType: "user", TargetID: f.user.ID, Reason: "explicit selected evidence", RequestID: newID(), EvidenceIDs: []string{object}}
	deletionBody(t, uiRegressionCall(f.nodes["governance"], "POST", "/api/reports", token, bad), 404)
	if pcCount(t, f.ctx, f.nodes["governance"], `SELECT count(*) FROM governance.reports`) != 0 {
		t.Fatal("foreign upload partially created a report")
	}
	request := reportCreateRequest{TargetType: "user", TargetID: peer.ID, Reason: "explicit selected evidence", RequestID: newID(), EvidenceIDs: []string{object}, SelectedMessageIDs: []string{message["id"].(string)}}
	created := deletionBody(t, uiRegressionCall(f.nodes["governance"], "POST", "/api/reports", f.token, request), 200)
	report := created["id"].(string)
	deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", "/api/reports/"+report, token, nil), 404)
	deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", "/api/admin/reports/"+report, token, nil), 403)
	view := deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", "/api/admin/reports/"+report, adminToken, nil), 200)
	if len(view["evidence"].([]any)) != 1 || len(view["selectedMessages"].([]any)) != 1 || view["selectedMessages"].([]any)[0].(map[string]any)["text"] != "selected current source" {
		t.Fatal("explicit materials missing or automatic whole chat supplied")
	}
	for _, filter := range []string{"reporterId=" + f.user.ID, "targetUserId=" + peer.ID} {
		list := deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", "/api/admin/reports?"+filter, adminToken, nil), 200)
		if len(list["items"].([]any)) != 1 {
			t.Fatal("official report filter lost matching case")
		}
	}
	deletionBody(t, uiRegressionCall(f.nodes["media"], "GET", "/api/reports/evidence/"+object, adminToken, nil), 404)
	download := uiRegressionCall(f.nodes["media"], "GET", "/api/reports/evidence/"+object+"?reportId="+report, adminToken, nil)
	if download.Code != 200 || !bytes.Equal(download.Body.Bytes(), imageBytes.Bytes()) {
		t.Fatal("report-bound official evidence read failed")
	}
	edit := map[string]string{"text": "new text not silently adopted", "expectedVersion": message["version"].(string), "requestId": newID()}
	deletionBody(t, uiRegressionCall(f.nodes["message"], "PATCH", "/api/messages/"+message["id"].(string), token, edit), 200)
	view = deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", "/api/admin/reports/"+report, adminToken, nil), 200)
	selected := view["selectedMessages"].([]any)[0].(map[string]any)
	if selected["available"] != false || selected["text"] != nil {
		t.Fatal("source edit revived or silently replaced selected content")
	}
}

func TestUIUsernameCurrentIdentityAndSnapshotRetractionPG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, peer, token)
	identity := f.nodes["identity"]
	old := f.user.Username
	next := "rename_" + strings.ReplaceAll(newID(), "-", "")[:18]
	q := usernameChangeRequest{Username: strings.ToUpper(next), ExpectedUsername: old, RequestID: newID()}
	first := deletionBody(t, uiRegressionCall(identity, "PATCH", "/api/me/username", f.token, q), 200)
	if first["current"] != true || first["user"].(map[string]any)["id"] != f.user.ID || first["user"].(map[string]any)["username"] != next {
		t.Fatal("rename changed identity or lacked authoritative current user")
	}
	deletionBody(t, uiRegressionCall(identity, "PATCH", "/api/me/username", f.token, q), 200)
	if pcCount(t, f.ctx, identity, `SELECT count(*) FROM auth.sessions WHERE user_id=$1 AND revoked_at IS NULL`, f.user.ID) != 2 {
		t.Fatal("rename retired existing families")
	}
	if pcCount(t, f.ctx, identity, `SELECT count(*) FROM auth.identity_reservations WHERE username=$1 AND user_id=$2`, old, f.user.ID) != 1 {
		t.Fatal("old username became reusable by another identity")
	}
	bad := q
	bad.RequestID = newID()
	bad.Username = peer.Username
	bad.ExpectedUsername = next
	deletionBody(t, uiRegressionCall(identity, "PATCH", "/api/me/username", f.token, bad), 409)
	newer := usernameChangeRequest{Username: "new_" + strings.ReplaceAll(newID(), "-", "")[:20], ExpectedUsername: next, RequestID: newID()}
	deletionBody(t, uiRegressionCall(identity, "PATCH", "/api/me/username", f.token, newer), 200)
	prior := deletionBody(t, uiRegressionCall(identity, "PATCH", "/api/me/username", f.token, q), 200)
	if prior["current"] != false || prior["user"].(map[string]any)["username"] != newer.Username {
		t.Fatal("old original response rolled current username back")
	}
	c := uiRegressionDirect(t, f, peer)
	m := uiRegressionSend(t, f, f.user.ID, f.token, c["id"].(string), "original selected text")
	edit := deletionBody(t, uiRegressionCall(f.nodes["message"], "PATCH", "/api/messages/"+m["id"].(string), f.token, map[string]string{"text": "other-device edit", "expectedVersion": m["version"].(string), "requestId": newID()}), 200)
	retract := messageRetractionRequest{ExpectedVersion: m["version"].(string), RequestID: newID()}
	deletionBody(t, uiRegressionCall(f.nodes["message"], "DELETE", "/api/messages/"+m["id"].(string), f.token, retract), 409)
	retract.ExpectedVersion = edit["version"].(string)
	deletionBody(t, uiRegressionCall(f.nodes["message"], "DELETE", "/api/messages/"+m["id"].(string), f.token, retract), 200)
	retract.ExpectedVersion = m["version"].(string)
	deleted := deletionBody(t, uiRegressionCall(f.nodes["message"], "DELETE", "/api/messages/"+m["id"].(string), f.token, retract), 200)
	if deleted["deleted"] != true || deleted["text"] != "" {
		t.Fatal("original retraction replay revived selected text")
	}
}

func TestUIReportSelectedAttachmentAndMissingEvidencePG(t *testing.T) {
	f := newDeletionPG(t, 7)
	peer, token := uiRegressionPeer(t, f)
	uiRegressionFriend(t, f, peer, token)
	admin, adminToken := uiRegressionPeer(t, f)
	deletionExec(t, f.ctx, f.nodes["identity"], `UPDATE auth.users SET admin=true WHERE id=$1`, admin.ID)
	c := uiRegressionDirect(t, f, peer)
	conversation := c["id"].(string)
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	q, _ := mediaUploadPGRequest(t, f, imageBytes.Bytes())
	q.Name = "selected-chat.png"
	chat := deletionBody(t, mediaUploadPGCall(t, f.nodes["media"], f.token, &q, q.Name, imageBytes.Bytes()), 200)
	send := sendPayload{ClientMsgID: newID(), Type: "image", AttachmentID: chat["id"].(string), RestoreEpoch: "1", SendAdmission: c["sendAdmission"].(string)}
	message := deletionBody(t, uiRegressionCall(f.nodes["message"], "POST", "/api/conversations/"+conversation+"/messages", f.token, send), 200)
	// This fixture verifies the existing container classifier and real upload bytes,
	// not encoded-video playback or a camera capture.
	video := webmTestFile(false, webmTestElement(webmTracksID, webmTestTrack(1, 1, "V_VP8")), webmTestElement(webmCluster, webmTestUint(0xe7, 0)))
	videoRequest, _ := mediaUploadPGRequest(t, f, video)
	videoRequest.Name = "explicit-selected.webm"
	evidence := deletionBody(t, uiRegressionEvidenceUpload(t, f, video, videoRequest), 200)
	if evidence["contentType"] != "video/webm" {
		t.Fatal("video evidence bypassed actual container classification")
	}
	report := deletionBody(t, uiRegressionCall(f.nodes["governance"], "POST", "/api/reports", f.token, reportCreateRequest{TargetType: "user", TargetID: peer.ID, Reason: "selected attachment only", RequestID: newID(), EvidenceIDs: []string{evidence["id"].(string)}, SelectedMessageIDs: []string{message["id"].(string)}}), 200)
	path := "/api/admin/reports/" + report["id"].(string)
	view := deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", path, adminToken, nil), 200)
	selected := view["selectedMessages"].([]any)[0].(map[string]any)
	attachment := selected["attachmentURL"].(string)
	deletionBody(t, uiRegressionCall(f.nodes["media"], "GET", attachment, token, nil), 403)
	read := uiRegressionCall(f.nodes["media"], "GET", attachment, adminToken, nil)
	if read.Code != 200 || !bytes.Equal(read.Body.Bytes(), imageBytes.Bytes()) || read.Header().Get("Cache-Control") != "private,no-store" {
		t.Fatal("selected report attachment did not use authenticated original bytes")
	}
	deletionBody(t, uiRegressionCall(f.nodes["message"], "DELETE", "/api/messages/"+message["id"].(string), f.token, nil), 200)
	deletionBody(t, uiRegressionCall(f.nodes["media"], "GET", attachment, adminToken, nil), 410)
	if err := os.Remove(filepath.Join(f.nodes["media"].Config.StateDir, "Media", evidence["id"].(string))); err != nil {
		t.Fatal(err)
	}
	view = deletionBody(t, uiRegressionCall(f.nodes["governance"], "GET", path, adminToken, nil), 200)
	file := view["evidence"].([]any)[0].(map[string]any)
	selected = view["selectedMessages"].([]any)[0].(map[string]any)
	if file["available"] != false || file["url"] != nil || selected["available"] != false || selected["attachmentURL"] != nil || selected["createdAt"] != nil {
		t.Fatal("missing bytes or retracted source was presented as available")
	}
}
