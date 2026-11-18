package jobsim

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nats-io/nats.go"
)

const syncNATSWireLimit = 4096

var errSyncNATSUnavailable = errors.New("sync transport unavailable")
var errSyncNATSConfiguration = errors.New("sync transport configuration mismatch")
var errSyncNATSReference = errors.New("sync transport reference rejected")

// Called inside the owner's migration transaction, before runtime grants.
func migrateSyncPublication(ctx context.Context, tx pgx.Tx, owner string) error {
	if !syncNATSOwner(owner) {
		return fmt.Errorf("unsupported sync publication owner")
	}
	schema := pgx.Identifier{owner}.Sanitize()
	_, err := tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS "+schema+".sync_publication(singleton boolean PRIMARY KEY CHECK(singleton),revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0)); INSERT INTO "+schema+".sync_publication(singleton,revision) VALUES(true,0) ON CONFLICT DO NOTHING")
	return err
}

func syncNATSOwner(owner string) bool {
	switch owner {
	case "auth", "directory", "conversation", "message", "sync":
		return true
	}
	return false
}

func syncNATSPositive(value string) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number > 0 && strconv.FormatInt(number, 10) == value
}

func syncNATSID(value string) bool {
	return validObjectID(value) && value == strings.ToLower(value)
}

func syncNATSNames(scope string) (string, string) {
	return "JSIM_" + strings.ReplaceAll(scope, "-", ""), "jobsim." + scope + ".>"
}

func syncNATSValidate(wire syncWireEvent, subject, scope, epoch string) error {
	ref := wire.Ref
	if !syncNATSOwner(wire.Owner) || !syncNATSID(scope) || wire.ScopeID != scope || !syncNATSID(wire.OriginEventID) || !syncNATSPositive(wire.Revision) || !syncNATSPositive(wire.RestoreEpoch) || wire.RestoreEpoch != epoch || ref.Owner != wire.Owner || ref.Revision != wire.Revision || ref.RevisionEpoch != epoch || !syncNATSID(ref.ID) || subject != "jobsim."+scope+"."+epoch+"."+wire.Owner || ref.State != "active" && ref.State != "removed" {
		return errSyncNATSReference
	}
	for _, id := range []string{ref.SubjectUserID, ref.ConversationID, ref.TargetUserID} {
		if id != "" && !syncNATSID(id) {
			return errSyncNATSReference
		}
	}
	switch wire.Owner {
	case "auth":
		if (ref.Kind != "account" && ref.Kind != "devices") || ref.SubjectUserID != ref.ID || ref.ConversationID != "" || ref.TargetUserID != "" || ref.Kind == "devices" && ref.State != "active" {
			return errSyncNATSReference
		}
	case "directory":
		if ref.ConversationID != "" || ref.SubjectUserID == "" {
			return errSyncNATSReference
		}
		switch ref.Kind {
		case "profile", "preferences":
			if ref.SubjectUserID != ref.ID || ref.TargetUserID != "" {
				return errSyncNATSReference
			}
		case "contacts", "relationship":
			if ref.SubjectUserID != ref.ID || ref.TargetUserID == "" || ref.TargetUserID == ref.SubjectUserID || ref.State != "active" {
				return errSyncNATSReference
			}
		case "contact_request":
			if ref.TargetUserID == "" || ref.TargetUserID == ref.SubjectUserID {
				return errSyncNATSReference
			}
		default:
			return errSyncNATSReference
		}
	case "conversation":
		if (ref.Kind != "conversation" && ref.Kind != "announcement") || ref.ConversationID != ref.ID || ref.SubjectUserID != "" {
			return errSyncNATSReference
		}
	case "message":
		if ref.Kind != "message" || ref.SubjectUserID == "" || ref.ConversationID == "" || ref.TargetUserID != "" {
			return errSyncNATSReference
		}
	case "sync":
		if ref.Owner != syncKindOwner(ref.Kind) || ref.SubjectUserID == "" || ref.ConversationID == "" || ref.TargetUserID != "" {
			return errSyncNATSReference
		}
	}
	return nil
}

func syncNATSToken(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 4096 {
		return "", errSyncNATSConfiguration
	}
	file, err := os.Open(path)
	if err != nil {
		return "", errSyncNATSConfiguration
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || !syncPrivateTokenFile(opened, file) {
		return "", errSyncNATSConfiguration
	}
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	defer func() {
		for i := range data {
			data[i] = 0
		}
	}()
	if err != nil || len(data) == 0 || len(data) > 4096 {
		return "", errSyncNATSConfiguration
	}
	data = bytes.TrimSuffix(data, []byte{'\n'})
	data = bytes.TrimSuffix(data, []byte{'\r'})
	if len(data) == 0 {
		return "", errSyncNATSConfiguration
	}
	for _, value := range data {
		if value < 33 || value > 126 {
			return "", errSyncNATSConfiguration
		}
	}
	return string(data), nil
}

func (a *App) syncNATSConnect(ctx context.Context) (*nats.Conn, nats.JetStreamContext, string, error) {
	address, err := url.Parse(a.Config.NATSURL)
	if err != nil || address.Hostname() == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Path != "" && address.Path != "/" || address.Scheme != "nats" && address.Scheme != "tls" || !syncNATSID(a.Config.DeploymentID) {
		return nil, nil, "", errSyncNATSConfiguration
	}
	loopback := strings.EqualFold(address.Hostname(), "localhost")
	if ip := net.ParseIP(address.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if !loopback && address.Scheme != "tls" {
		return nil, nil, "", errSyncNATSConfiguration
	}
	token, err := syncNATSToken(a.Config.NATSTokenFile)
	if err != nil {
		return nil, nil, "", err
	}
	options := []nats.Option{
		nats.Name("JobsIM sync transport"), nats.Timeout(3 * time.Second), nats.NoReconnect(),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, _ error) {}),
	}
	if token != "" {
		options = append(options, nats.Token(token))
	}
	if address.Scheme == "tls" {
		options = append(options, nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: address.Hostname()}))
	}
	connection, err := nats.Connect(address.String(), options...)
	if err != nil {
		return nil, nil, "", errSyncNATSUnavailable
	}
	js, err := connection.JetStream(nats.MaxWait(5 * time.Second))
	if err != nil {
		connection.Close()
		return nil, nil, "", errSyncNATSUnavailable
	}
	name, subject := syncNATSNames(a.Config.DeploymentID)
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := js.StreamInfo(name, nats.Context(probe))
	if errors.Is(err, nats.ErrStreamNotFound) {
		_, err = js.AddStream(&nats.StreamConfig{Name: name, Subjects: []string{subject}, Retention: nats.LimitsPolicy, Storage: nats.FileStorage, MaxAge: 7 * 24 * time.Hour, MaxBytes: 256 * 1024 * 1024, Replicas: 1, Discard: nats.DiscardOld}, nats.Context(probe))
		// Another owner may create the same fixed stream concurrently.
		if err == nil || connection.IsConnected() {
			info, err = js.StreamInfo(name, nats.Context(probe))
		}
	}
	if err != nil || info == nil {
		connection.Close()
		return nil, nil, "", errSyncNATSUnavailable
	}
	configuration := info.Config
	if configuration.Name != name || len(configuration.Subjects) != 1 || configuration.Subjects[0] != subject || configuration.Retention != nats.LimitsPolicy || configuration.Storage != nats.FileStorage || configuration.MaxAge != 7*24*time.Hour || configuration.MaxBytes != 256*1024*1024 || configuration.Replicas != 1 || configuration.Discard != nats.DiscardOld || configuration.NoAck || configuration.Sealed || configuration.Mirror != nil || len(configuration.Sources) != 0 || configuration.Template != "" || configuration.SubjectTransform != nil || configuration.RePublish != nil || configuration.AllowRollup || configuration.AllowMsgTTL || configuration.SubjectDeleteMarkerTTL != 0 {
		connection.Close()
		return nil, nil, "", errSyncNATSConfiguration
	}
	return connection, js, name, nil
}

func syncNATSWait(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func syncNATSRollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}

// StartSyncWorkers does not advertise readiness. A connection or configuration
// failure leaves owner Outbox and consumer facts intact while this worker retries.
func (a *App) StartSyncWorkers(ctx context.Context) {
	if a.DB == nil || a.Config.NATSURL == "" {
		return
	}
	owner := syncOwnerSchema(a.Role)
	if owner == "" && a.Role != "sync" {
		return
	}
	if owner != "" {
		go a.syncNATSWorker(ctx, owner)
	}
	if a.Role == "sync" {
		go a.syncNATSWorker(ctx, "")
	}
}

func (a *App) syncNATSWorker(ctx context.Context, owner string) {
	connected := &a.syncPublishConnected
	if owner == "" {
		connected = &a.syncConsumeConnected
	}
	setConnected := func(value bool) {
		a.syncTransportMu.Lock()
		defer a.syncTransportMu.Unlock()
		connected.Store(value)
		if a.Role == "sync" {
			a.syncTransportConnected.Store(a.syncPublishConnected.Load() && a.syncConsumeConnected.Load())
		} else {
			a.syncTransportConnected.Store(a.syncPublishConnected.Load())
		}
	}
	warned := false
	for ctx.Err() == nil {
		connection, js, stream, err := a.syncNATSConnect(ctx)
		if err == nil {
			setConnected(true)
			warned = false
			if owner != "" {
				err = a.syncNATSPublishLoop(ctx, connection, js, stream, owner)
			} else {
				err = a.syncNATSConsumeLoop(ctx, connection, js, stream)
			}
			connection.Close()
		}
		setConnected(false)
		if ctx.Err() != nil {
			return
		}
		if !warned {
			if errors.Is(err, errSyncNATSConfiguration) {
				log.Print("JobsIM sync transport configuration mismatch; retaining progress and retrying")
			} else {
				log.Print("JobsIM sync transport unavailable; retaining progress and retrying")
			}
			warned = true
		}
		if !syncNATSWait(ctx, 3*time.Second) {
			return
		}
	}
}

func (a *App) syncNATSPublishLoop(ctx context.Context, connection *nats.Conn, js nats.JetStreamContext, stream, owner string) error {
	for ctx.Err() == nil && connection.IsConnected() {
		published, err := a.syncNATSPublishOne(ctx, js, stream, owner)
		if err != nil {
			return err
		}
		if !published && !syncNATSWait(ctx, 250*time.Millisecond) {
			return ctx.Err()
		}
	}
	return errSyncNATSUnavailable
}

func (a *App) syncNATSPublishOne(parent context.Context, js nats.JetStreamContext, stream, owner string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return false, errSyncNATSUnavailable
	}
	defer syncNATSRollback(tx)
	schema := pgx.Identifier{owner}.Sanitize()
	var previous int64
	err = tx.QueryRow(ctx, "SELECT revision FROM "+schema+".sync_publication WHERE singleton FOR UPDATE SKIP LOCKED").Scan(&previous)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errSyncNATSUnavailable
	}
	wire := syncWireEvent{Owner: owner, Ref: syncRef{Owner: owner}}
	var revision int64
	err = tx.QueryRow(ctx, "SELECT revision,origin_event_id::text,scope_id::text,restore_epoch::text,kind,object_id::text,revision_epoch::text,COALESCE(subject_user_id::text,''),COALESCE(conversation_id::text,''),COALESCE(target_user_id::text,''),state FROM "+schema+".sync_outbox WHERE revision>$1 ORDER BY revision LIMIT 1", previous).Scan(&revision, &wire.OriginEventID, &wire.ScopeID, &wire.RestoreEpoch, &wire.Ref.Kind, &wire.Ref.ID, &wire.Ref.RevisionEpoch, &wire.Ref.SubjectUserID, &wire.Ref.ConversationID, &wire.Ref.TargetUserID, &wire.Ref.State)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, errSyncNATSUnavailable
	}
	wire.Revision, wire.Ref.Revision = strconv.FormatInt(revision, 10), strconv.FormatInt(revision, 10)
	subject := "jobsim." + a.Config.DeploymentID + "." + wire.RestoreEpoch + "." + owner
	if syncNATSValidate(wire, subject, a.Config.DeploymentID, wire.RestoreEpoch) != nil {
		return false, errSyncNATSReference
	}
	data, err := json.Marshal(wire)
	if err != nil || len(data) > syncNATSWireLimit {
		return false, errSyncNATSReference
	}
	ack, err := js.Publish(subject, data, nats.MsgId(wire.OriginEventID), nats.Context(ctx))
	if err != nil || ack == nil || ack.Stream != stream || ack.Sequence == 0 {
		return false, errSyncNATSUnavailable
	}
	result, err := tx.Exec(ctx, "UPDATE "+schema+".sync_publication SET revision=$1 WHERE singleton AND revision=$2", revision, previous)
	if err != nil || result.RowsAffected() != 1 || tx.Commit(ctx) != nil {
		return false, errSyncNATSUnavailable
	}
	return true, nil
}

func (a *App) syncNATSConsumeLoop(ctx context.Context, connection *nats.Conn, js nats.JetStreamContext, stream string) error {
	const durable = "JSIM_SYNC"
	_, subject := syncNATSNames(a.Config.DeploymentID)
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	info, err := js.ConsumerInfo(stream, durable, nats.Context(probe))
	if errors.Is(err, nats.ErrConsumerNotFound) {
		_, err = js.AddConsumer(stream, &nats.ConsumerConfig{Durable: durable, FilterSubject: subject, AckPolicy: nats.AckExplicitPolicy, DeliverPolicy: nats.DeliverAllPolicy, ReplayPolicy: nats.ReplayInstantPolicy, AckWait: 30 * time.Second, MaxAckPending: 128, MaxDeliver: -1}, nats.Context(probe))
		if err == nil || connection.IsConnected() {
			info, err = js.ConsumerInfo(stream, durable, nats.Context(probe))
		}
	}
	cancel()
	if err != nil || info == nil {
		return errSyncNATSUnavailable
	}
	configuration := info.Config
	if info.Name != durable || configuration.Durable != durable || configuration.FilterSubject != subject || len(configuration.FilterSubjects) != 0 || configuration.DeliverSubject != "" || configuration.AckPolicy != nats.AckExplicitPolicy || configuration.DeliverPolicy != nats.DeliverAllPolicy || configuration.ReplayPolicy != nats.ReplayInstantPolicy || configuration.AckWait != 30*time.Second || configuration.MaxAckPending != 128 || configuration.MaxDeliver != -1 || configuration.MemoryStorage || configuration.HeadersOnly || len(configuration.BackOff) != 0 || configuration.InactiveThreshold != 0 {
		return errSyncNATSConfiguration
	}
	subscription, err := js.PullSubscribe(subject, durable, nats.Bind(stream, durable))
	if err != nil {
		return errSyncNATSUnavailable
	}
	defer subscription.Unsubscribe()
	for ctx.Err() == nil && connection.IsConnected() {
		fetch, stop := context.WithTimeout(ctx, 1500*time.Millisecond)
		messages, fetchErr := subscription.Fetch(32, nats.Context(fetch))
		stop()
		if fetchErr != nil && !errors.Is(fetchErr, nats.ErrTimeout) && !errors.Is(fetchErr, context.DeadlineExceeded) {
			return errSyncNATSUnavailable
		}
		for _, message := range messages {
			operation, done := context.WithTimeout(ctx, 5*time.Second)
			err := a.syncNATSStore(operation, message)
			if err == nil {
				_ = message.AckSync(nats.Context(operation))
			} else if errors.Is(err, errSyncNATSReference) {
				_ = message.Term()
				log.Print("JobsIM sync transport rejected an invalid reference; no sync progress was created")
			} else {
				_ = message.NakWithDelay(time.Second)
			}
			done()
		}
	}
	return errSyncNATSUnavailable
}

func (a *App) syncNATSStore(ctx context.Context, message *nats.Msg) error {
	if message == nil || len(message.Data) == 0 || len(message.Data) > syncNATSWireLimit {
		return errSyncNATSReference
	}
	decoder := json.NewDecoder(bytes.NewReader(message.Data))
	decoder.DisallowUnknownFields()
	var wire syncWireEvent
	if decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF || message.Header.Get(nats.MsgIdHdr) != wire.OriginEventID {
		return errSyncNATSReference
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return errSyncNATSUnavailable
	}
	defer syncNATSRollback(tx)
	var scope, epoch string
	if tx.QueryRow(ctx, "SELECT deployment_id,restore_epoch::text FROM sync.instance WHERE singleton FOR SHARE").Scan(&scope, &epoch) != nil {
		return errSyncNATSUnavailable
	}
	if scope != a.Config.DeploymentID {
		return errSyncNATSConfiguration
	}
	if syncNATSValidate(wire, message.Subject, scope, wire.RestoreEpoch) != nil {
		return errSyncNATSReference
	}
	current, epochErr := strconv.ParseInt(epoch, 10, 64)
	if epochErr != nil || current < 1 {
		return errSyncNATSConfiguration
	}
	incoming, _ := strconv.ParseInt(wire.RestoreEpoch, 10, 64)
	if incoming > current {
		// A new owner epoch may arrive before the sync restore gate is activated.
		return errSyncNATSUnavailable
	}
	if wire.RestoreEpoch != epoch {
		return errSyncNATSReference
	}
	if err = a.syncStoreOrigin(ctx, tx, wire); err != nil {
		var api *apiError
		if errors.As(err, &api) && (api.Code == "SYNC_ORIGIN_CONFLICT" || api.Code == "SYNC_OWNER_MISMATCH") {
			return errSyncNATSReference
		}
		return errSyncNATSUnavailable
	}
	if tx.Commit(ctx) != nil {
		return errSyncNATSUnavailable
	}
	return nil
}
