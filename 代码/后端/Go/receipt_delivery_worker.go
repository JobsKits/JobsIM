package jobsim

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (a *App) StartReceiptDeliveryWorkers(ctx context.Context) {
	if a.Role != "sync" {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				attempt, cancel := context.WithTimeout(ctx, 8*time.Second)
				_ = a.refreshReceiptDeliveries(attempt, 32)
				cancel()
			}
		}
	}()
}

func (a *App) refreshReceiptDeliveries(ctx context.Context, limit int) error {
	rows, err := a.DB.Query(ctx, `SELECT manifest FROM sync.delivery_manifests m JOIN sync.instance i ON i.singleton AND i.restore_epoch=m.restore_epoch WHERE m.state IN('waiting','complete') ORDER BY m.last_attempt_at NULLS FIRST,m.expires_at,m.message_id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	items := []receiptDeliveryManifest{}
	for rows.Next() {
		var raw []byte
		var item receiptDeliveryManifest
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		if json.Unmarshal(raw, &item) != nil || !validReceiptManifest(item) {
			rows.Close()
			return fail(503, "DELIVERY_MANIFEST_CORRUPT", "Cannot advance an unverifiable original target range")
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err = a.DB.Exec(ctx, `UPDATE sync.delivery_manifests SET last_attempt_at=clock_timestamp() WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, item.Message, item.Generation); err != nil {
			return err
		}
		fn := a.withAdmissionFence("POST /api/sync/receipt-refresh", func(_ http.ResponseWriter, r *http.Request) error { return a.refreshReceiptDelivery(r.Context(), item) })
		request, _ := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1/receipt-refresh", nil)
		if err = fn(nil, request); err != nil {
			return err
		}
	}
	return nil
}

type receiptDeliveryCheckpoint struct {
	Current    bool   `json:"current"`
	State      string `json:"state"`
	Attachment string `json:"attachmentId"`
	Sender     string `json:"senderId"`
}

func (a *App) refreshReceiptDelivery(ctx context.Context, manifest receiptDeliveryManifest) error {
	if !manifest.Expires.After(time.Now()) {
		if err := a.publishReceiptCompletion(ctx, manifest); err != nil {
			return err
		}
		_, err := a.DB.Exec(ctx, `UPDATE sync.delivery_manifests SET state='expired' WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, manifest.Message, manifest.Generation)
		return err
	}
	var current receiptDeliveryCheckpoint
	if err := a.rpc(ctx, "message", "/internal/retention/checkpoint", manifest, &current); err != nil {
		return err
	}
	if !current.Current {
		if current.State != "superseded" {
			return nil
		}
		_, err := a.DB.Exec(ctx, `UPDATE sync.delivery_manifests SET state='superseded' WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, manifest.Message, manifest.Generation)
		return err
	}
	users := []string{}
	for _, user := range manifest.Audience {
		users = append(users, user.User)
	}
	var devices struct {
		Items []receiptDeviceState `json:"items"`
	}
	if err := a.rpc(ctx, "identity", "/internal/receipts/devices", map[string]any{"users": users, "restoreEpoch": manifest.Epoch}, &devices); err != nil {
		return err
	}
	revocations := map[string]string{}
	for _, target := range manifest.Targets {
		qualification, err := a.getStorageQualification(ctx, User{ID: target.User, DeviceID: target.Device, RestoreEpoch: manifest.Epoch}, target.Store)
		if err != nil {
			return err
		}
		if !qualification.Qualified || qualification.ID != target.Qualification || target.ObjectCapable && !qualification.ObjectQualified {
			revocations[target.User+":"+target.Device] = syncDigest([]byte("qualification:" + target.Qualification + ":" + qualification.ID))
		}
	}

	for _, device := range devices.Items {
		if !device.Active && device.Revocation != "" {
			revocations[device.User+":"+device.Device] = device.Revocation
		}
	}
	var audience struct {
		Users []receiptAudienceUser `json:"users"`
	}
	if err := a.rpc(ctx, "conversation", "/internal/receipts/audience", map[string]string{"conversationId": manifest.Conversation}, &audience); err != nil {
		return err
	}
	member := map[string]string{}
	for _, user := range audience.Users {
		member[user.User] = user.Membership
	}
	for _, user := range manifest.Audience {
		if member[user.User] != user.Membership {
			for _, target := range manifest.Targets {
				if target.User == user.User {
					revocations[target.User+":"+target.Device] = syncDigest([]byte("membership:" + manifest.Conversation + ":" + user.User + ":" + user.Membership + ":" + member[user.User]))
				}
			}
		}
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND generation=$3 AND manifest_digest=$4 FOR UPDATE`, a.Config.DeploymentID, manifest.Message, manifest.Generation, manifest.Digest).Scan(&state); err != nil {
		return err
	}
	if state != "waiting" && state != "complete" {
		return nil
	}
	for _, target := range manifest.Targets {
		key := target.User + ":" + target.Device
		if reason := revocations[key]; reason != "" {
			if _, err = tx.Exec(ctx, `UPDATE sync.delivery_targets SET revocation_event_id=COALESCE(revocation_event_id,$6) WHERE scope_id=$1 AND message_id=$2 AND generation=$3 AND user_id=$4 AND device_id=$5`, a.Config.DeploymentID, manifest.Message, manifest.Generation, target.User, target.Device, reason); err != nil {
				return err
			}
			continue
		}
		_, err = tx.Exec(ctx, `UPDATE sync.delivery_targets t SET content_received_at=COALESCE(t.content_received_at,(SELECT min(c.received_at) FROM sync.content_receipts c WHERE c.scope_id=t.scope_id AND c.message_id=t.message_id AND c.user_id=t.user_id AND c.device_id=t.device_id AND c.restore_epoch=$6 AND c.content_version=$7 AND c.store_generation=t.store_generation AND c.qualification_id=t.qualification_id AND c.result->>'deliveryGeneration'=$3::text AND c.result->>'manifestDigest'=$8)),
object_received_at=COALESCE(t.object_received_at,(SELECT min(o.received_at) FROM sync.object_receipts o JOIN sync.content_receipts c ON c.scope_id=o.scope_id AND c.source_context_id=o.source_context_id AND c.user_id=o.user_id AND c.device_id=o.device_id AND c.restore_epoch=o.restore_epoch AND c.request_id=o.parent_request_id WHERE o.scope_id=t.scope_id AND o.message_id=t.message_id AND o.user_id=t.user_id AND o.device_id=t.device_id AND o.restore_epoch=$6 AND o.content_version=$7 AND o.store_generation=t.store_generation AND o.qualification_id=t.qualification_id AND o.object_id::text=$9 AND c.result->>'deliveryGeneration'=$3::text AND c.result->>'manifestDigest'=$8))
WHERE t.scope_id=$1 AND t.message_id=$2 AND t.generation=$3 AND t.user_id=$4 AND t.device_id=$5 AND t.revocation_event_id IS NULL`, a.Config.DeploymentID, manifest.Message, manifest.Generation, target.User, target.Device, manifest.Epoch, manifest.Content, manifest.Digest, current.Attachment)
		if err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT user_id::text,device_id::text,content_received_at IS NOT NULL,object_received_at IS NOT NULL,revocation_event_id IS NOT NULL FROM sync.delivery_targets WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, manifest.Message, manifest.Generation)
	if err != nil {
		return err
	}
	received, objects, revoked := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var user, device string
		var body, object, removed bool
		if err = rows.Scan(&user, &device, &body, &object, &removed); err != nil {
			rows.Close()
			return err
		}
		key := user + ":" + device
		received[key], objects[key], revoked[key] = body, object, removed
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for kind, complete := range map[string]bool{"content": receiptRangeComplete(manifest.Audience, manifest.Targets, manifest.Scope, received, revoked, false), "object": current.Attachment != "" && receiptRangeComplete(manifest.Audience, manifest.Targets, manifest.Scope, objects, revoked, true)} {
		if !complete {
			continue
		}
		column := "receipt_complete_at"
		if kind == "object" {
			column = "object_complete_at"
		}
		_, err = tx.Exec(ctx, `UPDATE sync.delivery_manifests SET `+column+`=COALESCE(`+column+`,clock_timestamp()),state='complete' WHERE scope_id=$1 AND message_id=$2 AND generation=$3`, a.Config.DeploymentID, manifest.Message, manifest.Generation)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO sync.delivery_completions(scope_id,message_id,generation,kind,manifest_digest,completed_at) SELECT scope_id,message_id,generation,$4,manifest_digest,`+column+` FROM sync.delivery_manifests WHERE scope_id=$1 AND message_id=$2 AND generation=$3 ON CONFLICT DO NOTHING`, a.Config.DeploymentID, manifest.Message, manifest.Generation, kind)
		if err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return a.publishReceiptCompletion(ctx, manifest)
}

type receiptCompletion struct {
	Message    string    `json:"messageId"`
	Generation string    `json:"generation"`
	Kind       string    `json:"kind"`
	Digest     string    `json:"manifestDigest"`
	At         time.Time `json:"completedAt"`
}

func (a *App) publishReceiptCompletion(ctx context.Context, manifest receiptDeliveryManifest) error {
	rows, err := a.DB.Query(ctx, `SELECT kind,completed_at FROM sync.delivery_completions WHERE scope_id=$1 AND message_id=$2 AND generation=$3 AND NOT published`, a.Config.DeploymentID, manifest.Message, manifest.Generation)
	if err != nil {
		return err
	}
	items := []receiptCompletion{}
	for rows.Next() {
		item := receiptCompletion{Message: manifest.Message, Generation: manifest.Generation, Digest: manifest.Digest}
		if err = rows.Scan(&item.Kind, &item.At); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err = a.rpc(ctx, "message", "/internal/retention/complete", item, nil); err != nil {
			return err
		}
		if _, err = a.DB.Exec(ctx, `UPDATE sync.delivery_completions SET published=true WHERE scope_id=$1 AND message_id=$2 AND generation=$3 AND kind=$4 AND completed_at=$5`, a.Config.DeploymentID, item.Message, item.Generation, item.Kind, item.At); err != nil {
			return err
		}
	}
	return nil
}
