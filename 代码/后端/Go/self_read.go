package jobsim

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

func migrateSelfRead(ctx context.Context, tx pgx.Tx, scope string) error {
	if !validObjectID(scope) || strings.ToLower(scope) != scope {
		return fmt.Errorf("canonical deployment UUID required for personal read metadata")
	}
	work, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer work.Rollback(ctx)
	var deployment string
	if err = work.QueryRow(ctx, "SELECT deployment_id FROM sync.instance WHERE singleton FOR SHARE").Scan(&deployment); err != nil {
		return err
	}
	if deployment != scope {
		return fmt.Errorf("personal read metadata belongs to another deployment")
	}
	_, err = work.Exec(ctx, `CREATE TABLE IF NOT EXISTS sync.read_cursors(
id uuid NOT NULL UNIQUE,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),row_version bigint NOT NULL DEFAULT 0 CHECK(row_version>=0),membership_generation bigint NOT NULL CHECK(membership_generation>0),
read_sequence_epoch bigint NOT NULL DEFAULT 0 CHECK(read_sequence_epoch>=0),read_seq bigint NOT NULL DEFAULT 0 CHECK(read_seq>=0),CHECK((read_sequence_epoch=0)=(read_seq=0)),
continuity_status text NOT NULL DEFAULT 'not_evaluated',updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id));
CREATE TABLE IF NOT EXISTS sync.read_observations(
id uuid NOT NULL UNIQUE,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,
membership_generation bigint NOT NULL CHECK(membership_generation>0),message_id uuid NOT NULL,revision_epoch bigint NOT NULL CHECK(revision_epoch>0),row_version bigint NOT NULL DEFAULT 1 CHECK(row_version>0),
creation_epoch bigint,creation_sequence bigint,order_known boolean NOT NULL,eligible boolean NOT NULL,observed_at timestamptz NOT NULL DEFAULT now(),
CHECK((order_known AND creation_epoch IS NOT NULL AND creation_epoch>0 AND creation_sequence IS NOT NULL AND creation_sequence>0) OR(NOT order_known AND creation_epoch IS NULL AND creation_sequence IS NULL)),
PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id,membership_generation,message_id));
CREATE TABLE IF NOT EXISTS sync.read_operations(
id uuid NOT NULL UNIQUE,scope_id uuid NOT NULL,source_context_id text NOT NULL CHECK(source_context_id='personal'),user_id uuid NOT NULL,conversation_id uuid NOT NULL,
revision_epoch bigint NOT NULL CHECK(revision_epoch>0),row_version bigint NOT NULL DEFAULT 1 CHECK(row_version>0),request_id text NOT NULL CHECK(octet_length(request_id) BETWEEN 8 AND 128),
request_digest bytea NOT NULL CHECK(octet_length(request_digest)=32),result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND(result-ARRAY['revisionEpoch','version','membershipGeneration','readPoint','observedMessageIds','countStatus','continuityStatus','hasMoreEligible','hasUnorderedEligible']::text[])='{}'::jsonb),
created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(scope_id,source_context_id,user_id,conversation_id,revision_epoch,request_id));
CREATE INDEX IF NOT EXISTS self_read_observed_order ON sync.read_observations(scope_id,source_context_id,user_id,conversation_id,membership_generation,creation_epoch,creation_sequence) WHERE order_known AND eligible;`)
	if err != nil {
		return err
	}
	return work.Commit(ctx)
}

type selfReadPatch struct {
	RequestID            string   `json:"requestId"`
	RevisionEpoch        string   `json:"revisionEpoch"`
	MembershipGeneration string   `json:"membershipGeneration"`
	ObservedMessageIDs   []string `json:"observedMessageIds"`
}

type selfReadView struct {
	ID                   string    `json:"id"`
	ScopeID              string    `json:"scopeId"`
	SourceContextID      string    `json:"sourceContextId"`
	UserID               string    `json:"userId"`
	ConversationID       string    `json:"conversationId"`
	RevisionEpoch        string    `json:"revisionEpoch"`
	Version              string    `json:"version"`
	MembershipGeneration string    `json:"membershipGeneration"`
	ReadPoint            readPoint `json:"readPoint"`
	CountStatus          string    `json:"countStatus"`
	ContinuityStatus     string    `json:"continuityStatus"`
	Writable             bool      `json:"writable"`
	UnreadCount          *string   `json:"unreadCount,omitempty"`
	CountSourceWatermark string    `json:"countSourceWatermark,omitempty"`
}

type selfReadResult struct {
	RevisionEpoch        string    `json:"revisionEpoch"`
	Version              string    `json:"version"`
	MembershipGeneration string    `json:"membershipGeneration"`
	ReadPoint            readPoint `json:"readPoint"`
	ObservedMessageIDs   []string  `json:"observedMessageIds"`
	CountStatus          string    `json:"countStatus"`
	ContinuityStatus     string    `json:"continuityStatus"`
	HasMoreEligible      bool      `json:"hasMoreEligible"`
	HasUnorderedEligible bool      `json:"hasUnorderedEligible"`
}

type selfReadOperation struct {
	OperationID string         `json:"operationId"`
	Status      string         `json:"status"`
	Result      selfReadResult `json:"result"`
	Current     bool           `json:"current"`
}

type selfReadState struct {
	Exists               bool
	RevisionEpoch        string
	Version              int64
	MembershipGeneration string
	Point                readPoint
	ContinuityStatus     string
}

type selfReadQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type selfReadBoundaryLookup func(context.Context, readPointRequest) (readPointBoundaryResponse, error)

func selfReadPositive(value string) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number > 0 && str(number) == value
}

func validateSelfReadPatch(patch selfReadPatch) error {
	if len(patch.RequestID) < 8 || len(patch.RequestID) > 128 || !selfReadPositive(patch.RevisionEpoch) || !selfReadPositive(patch.MembershipGeneration) || patch.ObservedMessageIDs == nil || len(patch.ObservedMessageIDs) > 200 {
		return fail(400, "INVALID_READ_REQUEST", "Original request identity, exact epoch/generation and at most 200 observed IDs required")
	}
	seen := map[string]bool{}
	for _, id := range patch.ObservedMessageIDs {
		if !validObjectID(id) || strings.ToLower(id) != id || seen[id] {
			return fail(400, "INVALID_READ_REQUEST", "Observed message IDs must be canonical and unique")
		}
		seen[id] = true
	}
	return nil
}

func decodeSelfReadPatch(r *http.Request) (selfReadPatch, error) {
	var patch selfReadPatch
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&patch) != nil || decoder.Decode(new(any)) != io.EOF {
		return patch, fail(400, "INVALID_READ_REQUEST", "One bounded original read request required")
	}
	return patch, validateSelfReadPatch(patch)
}

func compareSelfReadPoints(first, second readPoint) (int, error) {
	firstEpoch, firstSequence, err := parseReadPoint(first)
	if err != nil {
		return 0, err
	}
	secondEpoch, secondSequence, err := parseReadPoint(second)
	if err != nil {
		return 0, err
	}
	if firstEpoch < secondEpoch || firstEpoch == secondEpoch && firstSequence < secondSequence {
		return -1, nil
	}
	if firstEpoch == secondEpoch && firstSequence == secondSequence {
		return 0, nil
	}
	return 1, nil
}

// Eligible is an owner-verified consecutive numeric page after current. A gap
// stops advancement; an unordered legacy candidate cannot certify a prefix.
func mergeSelfReadPrefix(current readPoint, eligible []readMessagePoint, observed map[string]bool, unordered bool) (readPoint, int, error) {
	if _, _, err := parseReadPoint(current); err != nil {
		return current, 0, err
	}
	previous, result, advanced := current, current, 0
	gap := unordered
	seen := map[string]bool{}
	for _, message := range eligible {
		point := readPoint{message.CreationEpoch, message.CreationSequence}
		comparison, err := compareSelfReadPoints(previous, point)
		if err != nil || comparison >= 0 || !message.OrderKnown || message.Deleted || !validObjectID(message.MessageID) || seen[message.MessageID] {
			return current, 0, invalidReadPoint()
		}
		seen[message.MessageID] = true
		previous = point
		if !observed[message.MessageID] {
			gap = true
		}
		if !gap {
			result, advanced = point, advanced+1
		}
	}
	return result, advanced, nil
}

// The caller's outer SH response fence covers membership and message RPCs.
// These routes never acquire a second host fence or hold PG locks across RPCs.
func (a *App) selfReadRoutes(handle func(string, func(http.ResponseWriter, *http.Request) error)) {
	handle("GET /api/conversations/{id}/read", func(w http.ResponseWriter, r *http.Request) error {
		u, membership, err := a.selfReadPrincipal(r)
		if err != nil {
			return err
		}
		view, err := a.selfReadLiveSummary(r.Context(), u, r.PathValue("id"), membership)
		if err != nil {
			return err
		}
		respond(w, 200, view)
		return nil
	})
	for _, resultOnly := range []bool{false, true} {
		pattern := "PUT /api/conversations/{id}/read"
		if resultOnly {
			pattern = "POST /api/conversations/{id}/read-results"
		}
		handle(pattern, func(w http.ResponseWriter, r *http.Request) error {
			u, membership, err := a.selfReadPrincipal(r)
			if err != nil {
				return err
			}
			patch, err := decodeSelfReadPatch(r)
			if err != nil {
				return err
			}
			var operation selfReadOperation
			if resultOnly {
				operation, err = a.selfReadOriginal(r.Context(), u, r.PathValue("id"), patch, membership)
			} else {
				lookup := func(ctx context.Context, request readPointRequest) (readPointBoundaryResponse, error) {
					var response readPointBoundaryResponse
					err := a.rpc(ctx, "message", "/internal/read-boundaries", request, &response)
					return response, err
				}
				operation, err = a.applySelfRead(r.Context(), u, r.PathValue("id"), patch, membership, lookup)
			}
			if err != nil {
				return err
			}
			respond(w, 200, operation)
			return nil
		})
	}
}

func (a *App) selfReadPrincipal(r *http.Request) (User, readPointMembership, error) {
	var membership readPointMembership
	if !validObjectID(r.PathValue("id")) || strings.ToLower(r.PathValue("id")) != r.PathValue("id") {
		return User{}, membership, fail(400, "INVALID_CONVERSATION", "Current personal conversation required")
	}
	u, err := a.syncUser(r)
	if err != nil {
		return u, membership, err
	}
	metadata, err := a.member(r.Context(), u.ID, r.PathValue("id"))
	if err != nil {
		return u, membership, err
	}
	membership, err = readPointMembershipFromMetadata(metadata)
	return u, membership, err
}

func (a *App) loadSelfReadState(ctx context.Context, db selfReadQuery, u User, conversation string) (selfReadState, error) {
	state := selfReadState{Point: readPoint{"0", "0"}, ContinuityStatus: "not_evaluated"}
	var scope, epoch string
	err := db.QueryRow(ctx, `SELECT i.deployment_id,i.restore_epoch::text,c.id IS NOT NULL,COALESCE(c.revision_epoch::text,''),COALESCE(c.row_version,0),COALESCE(c.membership_generation::text,''),COALESCE(c.read_sequence_epoch,0)::text,COALESCE(c.read_seq,0)::text,COALESCE(c.continuity_status,'not_evaluated')
FROM sync.instance i LEFT JOIN sync.read_cursors c ON c.scope_id=$1 AND c.source_context_id='personal' AND c.user_id=$2 AND c.conversation_id=$3 WHERE i.singleton`, a.Config.DeploymentID, u.ID, conversation).Scan(&scope, &epoch, &state.Exists, &state.RevisionEpoch, &state.Version, &state.MembershipGeneration, &state.Point.Epoch, &state.Point.Sequence, &state.ContinuityStatus)
	if err != nil {
		return state, err
	}
	if scope != a.Config.DeploymentID || epoch != u.RestoreEpoch {
		return state, fail(409, "RESTORE_EPOCH_CHANGED", "Personal read owner epoch changed")
	}
	return state, nil
}

func (a *App) selfReadView(u User, conversation string, membership readPointMembership, state selfReadState) selfReadView {
	view := selfReadView{ID: stableID(a.Config.Key, "self-read-cursor:"+a.Config.DeploymentID+":personal:"+u.ID+":"+conversation), ScopeID: a.Config.DeploymentID, SourceContextID: "personal", UserID: u.ID, ConversationID: conversation, RevisionEpoch: u.RestoreEpoch, Version: "0", MembershipGeneration: membership.Generation, ReadPoint: state.Point, CountStatus: "not_computed", ContinuityStatus: state.ContinuityStatus, Writable: true}
	if state.Exists && state.RevisionEpoch == u.RestoreEpoch {
		view.Version = str(state.Version)
	} else if state.Exists {
		view.ContinuityStatus = "needs_current_observation"
	}
	if state.Exists && state.MembershipGeneration != membership.Generation {
		view.ContinuityStatus = "needs_current_observation"
	}
	return view
}

// Hydration may reuse this private, current-ACL projection outside its PG row
// locks. It returns effective current-epoch version zero after a restore.
func (a *App) selfReadProjection(ctx context.Context, u User, conversation string) (selfReadView, error) {
	metadata, err := a.member(ctx, u.ID, conversation)
	if err != nil {
		return selfReadView{}, err
	}
	membership, err := readPointMembershipFromMetadata(metadata)
	if err != nil {
		return selfReadView{}, err
	}
	state, err := a.loadSelfReadState(ctx, a.DB, u, conversation)
	if err != nil {
		return selfReadView{}, err
	}
	return a.selfReadView(u, conversation, membership, state), nil
}

func (a *App) selfReadDigest(u User, conversation string, patch selfReadPatch) []byte {
	canonical, _ := json.Marshal(patch)
	return keyedDigest(a.Config.Key, "self-read-v1:"+a.Config.DeploymentID+":personal:"+u.ID+":"+conversation, canonical)
}

func (a *App) findSelfReadOperation(ctx context.Context, db selfReadQuery, u User, conversation string, patch selfReadPatch, digest []byte) (selfReadOperation, bool, error) {
	operation := selfReadOperation{Status: "applied"}
	var original, result []byte
	err := db.QueryRow(ctx, `SELECT id::text,request_digest,result FROM sync.read_operations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND revision_epoch=$4 AND request_id=$5`, a.Config.DeploymentID, u.ID, conversation, patch.RevisionEpoch, patch.RequestID).Scan(&operation.OperationID, &original, &result)
	if err == pgx.ErrNoRows {
		return operation, false, nil
	}
	if err != nil {
		return operation, false, err
	}
	if !hmac.Equal(original, digest) {
		return operation, true, fail(409, "IDEMPOTENCY_CONFLICT", "Original read observation request differs")
	}
	err = json.Unmarshal(result, &operation.Result)
	return operation, true, err
}

func selfReadOperationCurrent(operation selfReadOperation, u User, membership readPointMembership, state selfReadState) bool {
	return state.Exists && state.RevisionEpoch == u.RestoreEpoch && operation.Result.RevisionEpoch == state.RevisionEpoch && operation.Result.Version == str(state.Version) && operation.Result.MembershipGeneration == membership.Generation && state.MembershipGeneration == membership.Generation && operation.Result.ReadPoint == state.Point
}

// Result lookup proves only an existing original fact; it never revalidates IDs
// into a new write or substitutes a later cursor for the original result.
func (a *App) selfReadOriginal(ctx context.Context, u User, conversation string, patch selfReadPatch, membership readPointMembership) (selfReadOperation, error) {
	var operation selfReadOperation
	if err := validateSelfReadPatch(patch); err != nil {
		return operation, err
	}
	operation, found, err := a.findSelfReadOperation(ctx, a.DB, u, conversation, patch, a.selfReadDigest(u, conversation, patch))
	if err != nil {
		return operation, err
	}
	if !found {
		return operation, fail(404, "READ_OPERATION_NOT_FOUND", "No original applied read operation exists")
	}
	state, err := a.loadSelfReadState(ctx, a.DB, u, conversation)
	operation.Current = selfReadOperationCurrent(operation, u, membership, state)
	return operation, err
}

func selfReadBoundaryMatches(response readPointBoundaryResponse, u User, membership readPointMembership) error {
	if response.RestoreEpoch != u.RestoreEpoch {
		return fail(409, "RESTORE_EPOCH_CHANGED", "Message and personal read epochs do not agree")
	}
	if response.MembershipGeneration != membership.Generation || response.HistoryFloor != membership.HistoryFloor {
		return fail(409, "QUALIFICATION_CHANGED", "Current membership history changed")
	}
	return nil
}

func (a *App) selfReadThrough(ctx context.Context, u User, conversation string, membership readPointMembership, state selfReadState, points []readMessagePoint) (readPoint, error) {
	through := state.Point
	var stored readPoint
	err := a.DB.QueryRow(ctx, `SELECT creation_epoch::text,creation_sequence::text FROM sync.read_observations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND membership_generation=$4 AND order_known AND eligible ORDER BY creation_epoch DESC,creation_sequence DESC LIMIT 1`, a.Config.DeploymentID, u.ID, conversation, membership.Generation).Scan(&stored.Epoch, &stored.Sequence)
	if err != nil && err != pgx.ErrNoRows {
		return through, err
	}
	if err == nil {
		if comparison, err := compareSelfReadPoints(through, stored); err != nil {
			return through, err
		} else if comparison < 0 {
			through = stored
		}
	}
	for _, point := range points {
		if !point.OrderKnown || point.Deleted || point.SenderID == u.ID {
			continue
		}
		candidate := readPoint{point.CreationEpoch, point.CreationSequence}
		if comparison, err := compareSelfReadPoints(through, candidate); err != nil {
			return through, err
		} else if comparison < 0 {
			through = candidate
		}
	}
	return through, nil
}

// RPCs occur before the committing transaction. Local CAS retries collect a new
// page after concurrent writers rather than holding a row lock while calling message.
func (a *App) applySelfRead(ctx context.Context, u User, conversation string, patch selfReadPatch, membership readPointMembership, boundary selfReadBoundaryLookup) (selfReadOperation, error) {
	var operation selfReadOperation
	if err := validateSelfReadPatch(patch); err != nil {
		return operation, err
	}
	digest := a.selfReadDigest(u, conversation, patch)
	if original, found, err := a.findSelfReadOperation(ctx, a.DB, u, conversation, patch, digest); found || err != nil {
		if err != nil {
			return original, err
		}
		return a.selfReadOriginal(ctx, u, conversation, patch, membership)
	}
	if patch.RevisionEpoch != u.RestoreEpoch {
		return operation, fail(409, "RESTORE_EPOCH_CHANGED", "Uncommitted read request belongs to an old epoch")
	}
	if patch.MembershipGeneration != membership.Generation {
		return operation, fail(409, "QUALIFICATION_CHANGED", "Original membership generation is no longer current")
	}
	request := readPointRequest{UserID: u.ID, ConversationID: conversation, MessageIDs: patch.ObservedMessageIDs, After: readPoint{"0", "0"}, Through: readPoint{"0", "0"}, Limit: 200}
	observations, err := boundary(ctx, request)
	if err != nil {
		return operation, err
	}
	if err = selfReadBoundaryMatches(observations, u, membership); err != nil {
		return operation, err
	}
	if len(observations.Points) != len(patch.ObservedMessageIDs) {
		return operation, invalidReadPoint()
	}
	for index, point := range observations.Points {
		if point.MessageID != patch.ObservedMessageIDs[index] || !validObjectID(point.SenderID) {
			return operation, invalidReadPoint()
		}
		if point.OrderKnown {
			if _, _, err = parseReadPoint(readPoint{point.CreationEpoch, point.CreationSequence}); err != nil || point.CreationEpoch == "0" {
				return operation, invalidReadPoint()
			}
		} else if point.CreationEpoch != "unknown" || point.CreationSequence != "unknown" {
			return operation, invalidReadPoint()
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		state, err := a.loadSelfReadState(ctx, a.DB, u, conversation)
		if err != nil {
			return operation, err
		}
		through, err := a.selfReadThrough(ctx, u, conversation, membership, state, observations.Points)
		if err != nil {
			return operation, err
		}
		scan := readPointBoundaryResponse{NextEligible: []readMessagePoint{}, HasUnorderedEligible: observations.HasUnorderedEligible}
		if comparison, _ := compareSelfReadPoints(state.Point, through); comparison < 0 {
			request.MessageIDs, request.After, request.Through = nil, state.Point, through
			scan, err = boundary(ctx, request)
			if err != nil {
				return operation, err
			}
			if err = selfReadBoundaryMatches(scan, u, membership); err != nil {
				return operation, err
			}
		}
		observed := map[string]bool{}
		for _, id := range patch.ObservedMessageIDs {
			observed[id] = true
		}
		eligibleIDs := make([]string, len(scan.NextEligible))
		for index, point := range scan.NextEligible {
			comparison, pointError := compareSelfReadPoints(readPoint{point.CreationEpoch, point.CreationSequence}, through)
			if point.SenderID == u.ID || pointError != nil || comparison > 0 {
				return operation, invalidReadPoint()
			}
			eligibleIDs[index] = point.MessageID
		}
		if len(eligibleIDs) > 200 {
			return operation, invalidReadPoint()
		}
		if len(eligibleIDs) > 0 {
			rows, err := a.DB.Query(ctx, `SELECT message_id::text FROM sync.read_observations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND membership_generation=$4 AND message_id=ANY($5::uuid[])`, a.Config.DeploymentID, u.ID, conversation, membership.Generation, eligibleIDs)
			if err != nil {
				return operation, err
			}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					rows.Close()
					return operation, err
				}
				observed[id] = true
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return operation, err
			}
		}
		point, advanced, err := mergeSelfReadPrefix(state.Point, scan.NextEligible, observed, scan.HasUnorderedEligible)
		if err != nil {
			return operation, err
		}
		continuity := "complete_through_observed"
		if scan.HasUnorderedEligible {
			continuity = "unordered_messages"
		} else if advanced < len(scan.NextEligible) {
			continuity = "observation_gap"
		} else if scan.HasMore {
			continuity = "page_limited"
		}
		var retry bool
		operation, retry, err = a.commitSelfRead(ctx, u, conversation, patch, digest, membership, state, observations.Points, point, continuity, scan)
		if err != nil || !retry {
			return operation, err
		}
	}
	return operation, fail(503, "READ_STATE_BUSY", "Concurrent read observations changed; retry the original request")
}

func (a *App) commitSelfRead(ctx context.Context, u User, conversation string, patch selfReadPatch, digest []byte, membership readPointMembership, before selfReadState, points []readMessagePoint, prefix readPoint, continuity string, scan readPointBoundaryResponse) (selfReadOperation, bool, error) {
	var operation selfReadOperation
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return operation, false, err
	}
	defer tx.Rollback(ctx)
	var scope, epoch string
	if err = tx.QueryRow(ctx, "SELECT deployment_id,restore_epoch::text FROM sync.instance WHERE singleton FOR SHARE").Scan(&scope, &epoch); err != nil {
		return operation, false, err
	}
	if scope != a.Config.DeploymentID || epoch != u.RestoreEpoch {
		return operation, false, fail(409, "RESTORE_EPOCH_CHANGED", "Read owner epoch changed before commit")
	}
	cursorID := stableID(a.Config.Key, "self-read-cursor:"+scope+":personal:"+u.ID+":"+conversation)
	if _, err = tx.Exec(ctx, `INSERT INTO sync.read_cursors(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch,membership_generation) VALUES($1,$2,'personal',$3,$4,$5,$6) ON CONFLICT DO NOTHING`, cursorID, scope, u.ID, conversation, epoch, membership.Generation); err != nil {
		return operation, false, err
	}
	locked := selfReadState{Exists: true}
	if err = tx.QueryRow(ctx, `SELECT revision_epoch::text,row_version,membership_generation::text,read_sequence_epoch::text,read_seq::text,continuity_status FROM sync.read_cursors WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 FOR UPDATE`, scope, u.ID, conversation).Scan(&locked.RevisionEpoch, &locked.Version, &locked.MembershipGeneration, &locked.Point.Epoch, &locked.Point.Sequence, &locked.ContinuityStatus); err != nil {
		return operation, false, err
	}
	if original, found, lookupError := a.findSelfReadOperation(ctx, tx, u, conversation, patch, digest); found || lookupError != nil {
		if lookupError != nil {
			return original, false, lookupError
		}
		original.Current = selfReadOperationCurrent(original, u, membership, locked)
		return original, false, tx.Commit(ctx)
	}
	unchanged := before.Exists && before == locked || !before.Exists && locked.RevisionEpoch == epoch && locked.Version == 0 && locked.Point == (readPoint{"0", "0"})
	if !unchanged {
		return operation, true, nil
	}
	version := locked.Version
	if locked.RevisionEpoch != epoch {
		version = 0
	}
	if version == math.MaxInt64 {
		return operation, false, fail(409, "READ_VERSION_CONFLICT", "Read metadata version is exhausted")
	}
	for _, point := range points {
		if point.Deleted || point.SenderID == u.ID {
			continue
		}
		if point.OrderKnown {
			comparison, compareError := compareSelfReadPoints(readPoint{point.CreationEpoch, point.CreationSequence}, prefix)
			if compareError != nil {
				return operation, false, compareError
			}
			if comparison <= 0 {
				continue
			}
		}
		var creationEpoch, creationSequence any
		if point.OrderKnown {
			creationEpoch, creationSequence = point.CreationEpoch, point.CreationSequence
		}
		id := stableID(a.Config.Key, "self-read-observation:"+scope+":personal:"+u.ID+":"+conversation+":"+membership.Generation+":"+point.MessageID)
		_, err = tx.Exec(ctx, `INSERT INTO sync.read_observations(id,scope_id,source_context_id,user_id,conversation_id,membership_generation,message_id,revision_epoch,creation_epoch,creation_sequence,order_known,eligible) VALUES($1,$2,'personal',$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(scope_id,source_context_id,user_id,conversation_id,membership_generation,message_id) DO NOTHING`, id, scope, u.ID, conversation, membership.Generation, point.MessageID, epoch, creationEpoch, creationSequence, point.OrderKnown, !point.Deleted && point.SenderID != u.ID)
		if err != nil {
			return operation, false, err
		}
	}
	// Once a prefix proves these IDs read, retaining individual rows would grow
	// every future baseline without adding information. Holes/legacy IDs remain.
	if _, err = tx.Exec(ctx, `DELETE FROM sync.read_observations WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3 AND membership_generation=$4 AND order_known AND (creation_epoch,creation_sequence)<=($5::bigint,$6::bigint)`, scope, u.ID, conversation, membership.Generation, prefix.Epoch, prefix.Sequence); err != nil {
		return operation, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE sync.read_cursors SET revision_epoch=$4,row_version=$5,membership_generation=$6,read_sequence_epoch=$7,read_seq=$8,continuity_status=$9,updated_at=now() WHERE scope_id=$1 AND source_context_id='personal' AND user_id=$2 AND conversation_id=$3`, scope, u.ID, conversation, epoch, version+1, membership.Generation, prefix.Epoch, prefix.Sequence, continuity); err != nil {
		return operation, false, err
	}
	ids := append([]string{}, patch.ObservedMessageIDs...)
	operation = selfReadOperation{OperationID: stableID(a.Config.Key, "self-read-operation:"+scope+":personal:"+u.ID+":"+conversation+":"+patch.RevisionEpoch+":"+patch.RequestID), Status: "applied", Current: true, Result: selfReadResult{RevisionEpoch: epoch, Version: str(version + 1), MembershipGeneration: membership.Generation, ReadPoint: prefix, ObservedMessageIDs: ids, CountStatus: "not_computed", ContinuityStatus: continuity, HasMoreEligible: scan.HasMore, HasUnorderedEligible: scan.HasUnorderedEligible}}
	result, err := json.Marshal(operation.Result)
	if err != nil {
		return operation, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sync.read_operations(id,scope_id,source_context_id,user_id,conversation_id,revision_epoch,request_id,request_digest,result) VALUES($1,$2,'personal',$3,$4,$5,$6,$7,$8)`, operation.OperationID, scope, u.ID, conversation, epoch, patch.RequestID, digest, result); err != nil {
		return operation, false, err
	}
	return operation, false, tx.Commit(ctx)
}
