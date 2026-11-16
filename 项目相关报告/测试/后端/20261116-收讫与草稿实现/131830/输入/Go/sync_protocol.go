package jobsim

import "time"

// Synchronization persists references only. Current bodies are projected through
// their owner on every response and never become a second sync body store.
type syncRef struct {
	Owner          string `json:"owner"`
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Revision       string `json:"revision"`
	RevisionEpoch  string `json:"revisionEpoch"`
	SubjectUserID  string `json:"subjectUserId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	TargetUserID   string `json:"targetUserId,omitempty"`
	State          string `json:"state"`
}

type syncWireEvent struct {
	Owner         string  `json:"owner"`
	OriginEventID string  `json:"originEventId"`
	ScopeID       string  `json:"scopeId"`
	RestoreEpoch  string  `json:"restoreEpoch"`
	Revision      string  `json:"revision"`
	Ref           syncRef `json:"ref"`
}

type syncOwnerCapture struct {
	Owner             string    `json:"owner"`
	RestoreEpoch      string    `json:"restoreEpoch"`
	Watermark         string    `json:"watermark"`
	PermissionVersion string    `json:"permissionVersion"`
	Refs              []syncRef `json:"refs"`
}

type syncProjection struct {
	Ref                  syncRef `json:"ref"`
	State                string  `json:"state"`
	BodyDigest           string  `json:"bodyDigest,omitempty"`
	BodyEnvelope         string  `json:"bodyEnvelope,omitempty"`
	Object               any     `json:"object,omitempty"`
	CurrentRevision      string  `json:"currentRevision,omitempty"`
	CurrentRevisionEpoch string  `json:"currentRevisionEpoch,omitempty"`
	RevisionMatch        bool    `json:"revisionMatch"`
}

func syncKindOwner(kind string) string {
	switch kind {
	case "account", "devices":
		return "auth"
	case "profile", "contacts", "relationship", "contact_request", "preferences":
		return "directory"
	case "conversation", "announcement":
		return "conversation"
	case "message":
		return "message"
	case "read_state", "read_observation", "conversation_view", "draft":
		return "sync"
	}
	return ""
}

type syncPageManifest struct {
	Page   int    `json:"page"`
	Count  int    `json:"count"`
	Digest string `json:"digest"`
}

type syncSnapshot struct {
	ID                      string             `json:"snapshotId"`
	ScopeID                 string             `json:"scopeId"`
	PrincipalID             string             `json:"principalId"`
	SourceContextID         string             `json:"sourceContextId"`
	DeviceID                string             `json:"deviceId"`
	StoreGeneration         string             `json:"storeGeneration"`
	RestoreEpoch            string             `json:"restoreEpoch"`
	StreamID                string             `json:"streamId"`
	SchemaVersion           string             `json:"schemaVersion"`
	Boundary                string             `json:"boundary"`
	BoundaryToken           string             `json:"boundaryToken"`
	OwnerWatermarks         map[string]string  `json:"ownerWatermarks"`
	PermissionGeneration    string             `json:"permissionGeneration"`
	ClassificationWatermark string             `json:"classificationWatermark"`
	Coverage                map[string]any     `json:"coverage"`
	PageManifest            []syncPageManifest `json:"pageManifest"`
	ExpiresAt               time.Time          `json:"expiresAt"`
}

var syncOwners = []string{"auth", "directory", "conversation", "message"}

// The schema selects a protocol stream, never a new account/device/store.
func syncSchemaIncludes(schema, kind string) bool {
	switch kind {
	case "preferences", "read_state", "read_observation", "conversation_view", "draft":
		return schema == "2"
	}
	return true
}

func syncSchemaOwners(schema string) []string {
	owners := append([]string{}, syncOwners...)
	if schema == "2" {
		owners = append(owners, "sync")
	}
	return owners
}

func syncOwnerSchema(role string) string {
	if role == "sync" {
		return role
	}
	if role == "identity" {
		return "auth"
	}
	for _, owner := range syncOwners {
		if role == owner {
			return role
		}
	}
	return ""
}

func syncOwnerRole(owner string) string {
	if owner == "auth" {
		return "identity"
	}
	return owner
}
