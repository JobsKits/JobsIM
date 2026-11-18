// Package jobsim owns the first executable PostgreSQL-backed domain services.
package jobsim

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

type User struct {
	CreatedAt            string           `json:"createdAt,omitempty"`
	Signature            string           `json:"signature"`
	ContactBindings      *ContactBindings `json:"contactBindings,omitempty"`
	RoleGeneration       string           `json:"roleGeneration,omitempty"`
	Role                 string           `json:"role,omitempty"`
	MembershipGeneration string           `json:"membershipGeneration,omitempty"`
	State                string           `json:"state,omitempty"`
	AvatarURL            string           `json:"avatarURL,omitempty"`
	AvatarThumbnailURL   string           `json:"avatarThumbnailURL,omitempty"`
	ProfileVersion       string           `json:"profileVersion,omitempty"`
	DeviceID             string           `json:"deviceId,omitempty"`
	RestoreEpoch         string           `json:"restoreEpoch,omitempty"`
	ID                   string           `json:"id"`
	Username             string           `json:"username"`
	DisplayName          string           `json:"displayName"`
	Admin                bool             `json:"admin,omitempty"`
}
type Conversation struct {
	RoleGeneration       string    `json:"roleGeneration"`
	MembershipGeneration string    `json:"membershipGeneration"`
	SendAdmission        string    `json:"sendAdmission"`
	State                string    `json:"state"`
	ReadOnly             bool      `json:"readOnly"`
	ID                   string    `json:"id"`
	Kind                 string    `json:"kind"`
	Title                string    `json:"title"`
	OwnerID              string    `json:"ownerId"`
	MemberIDs            []string  `json:"memberIds"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Cursor               string    `json:"cursor"`
}
type Message struct {
	retentionChecked      bool
	Forwarded             bool   `json:"forwarded,omitempty"`
	ForwardUnavailable    bool   `json:"forwardUnavailable,omitempty"`
	ForwardAdmission      string `json:"forwardAdmission,omitempty"`
	ReplyAdmission        string `json:"replyAdmission,omitempty"`
	AttachmentURL         string `json:"attachmentURL,omitempty"`
	sourcePending         bool
	forwardUncertain      bool
	forwardDepth          int
	forwardMembership     string
	forwardSecurity       string
	forwardEpoch          string
	forwardLeafMessage    string
	forwardLeafAttachment string
	ContentVersion        string     `json:"contentVersion"`
	MentionedUserIDs      []string   `json:"mentionedUserIds,omitempty"`
	MentionAll            bool       `json:"mentionAll,omitempty"`
	MentionedMe           bool       `json:"mentionedMe"`
	ContentUnavailable    bool       `json:"contentUnavailable,omitempty"`
	DeleteScope           string     `json:"deleteScope,omitempty"`
	DeletionGeneration    string     `json:"deletionGeneration,omitempty"`
	ID                    string     `json:"id"`
	ConversationID        string     `json:"conversationId"`
	SenderID              string     `json:"senderId"`
	ClientMsgID           string     `json:"clientMsgId"`
	Type                  string     `json:"type"`
	Text                  string     `json:"text"`
	ReplyToID             string     `json:"replyToId,omitempty"`
	AttachmentID          string     `json:"attachmentId,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	Version               string     `json:"version"`
	Sequence              string     `json:"sequence"`
	CreationEpoch         string     `json:"creationEpoch"`
	CreationSequence      string     `json:"creationSequence"`
	Deleted               bool       `json:"deleted"`
	Reactions             []Reaction `json:"reactions"`
	Pinned                bool       `json:"pinned"`
	Favorite              bool       `json:"favorite"`
}
type Reaction struct {
	Emoji  string `json:"emoji"`
	UserID string `json:"userId"`
}
type authResult struct {
	Token        string `json:"token,omitempty"`
	RefreshToken string `json:"refreshToken,omitempty"`
	User         User   `json:"user"`
	DeviceID     string `json:"deviceId"`
	RestoreEpoch string `json:"restoreEpoch"`
	sessionMetadata
}
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string                 { return e.Message }
func fail(status int, code, message string) error { return &apiError{status, code, message} }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
func decode(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fail(400, "INVALID_JSON", "Invalid JSON request")
	}
	return nil
}
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ae *apiError
	if !errors.As(err, &ae) {
		ae = &apiError{500, "INTERNAL", "Operation failed; retry using the original request identity"}
	}
	respond(w, ae.Status, map[string]any{"error": map[string]string{"code": ae.Code, "message": ae.Message}, "requestId": r.Header.Get("X-Request-ID")})
}
func str(v int64) string { return fmt.Sprintf("%d", v) }
