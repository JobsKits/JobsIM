package jobsim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSyncMigrationPGPoisonOwnerQueueStillRetiresOtherHolder(t *testing.T) {
	m := newMigrationPG(t)
	_, one := m.create(t)
	_, two := m.create(t)
	first, second := one["id"].(string), two["id"].(string)
	if first > second {
		first, second = second, first
	}
	poison, _ := m.app.migrationPath(first)
	if err := os.Remove(poison); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(poison, 0700); err != nil {
		t.Fatal(err)
	}
	owner := m.base.nodes["message"]
	operation := "poison-source:" + newID()
	deletionFence(t, owner, func(ctx context.Context) error {
		if err := owner.freezeMessageContentExportsAt(ctx, m.message.ID, operation, 1); err == nil {
			t.Fatal("poison holder falsely acknowledged a complete original source drain")
		}
		return nil
	})
	bad, err := m.app.readMigration(m.base.ctx, first)
	if err != nil || bad.State != "revoking" || len(bad.Key) != 0 {
		t.Fatal("poison holder retained delivery key", err)
	}
	good, err := m.app.readMigration(m.base.ctx, second)
	if err != nil || good.State != "retired" || len(good.Key) != 0 {
		t.Fatal("first poison starved another original holder", err)
	}
	path, _ := m.app.migrationPath(second)
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("later original capsule survived failed source drain", err)
	}
	var original string
	var pending bool
	if err = m.app.DB.QueryRow(m.base.ctx, `SELECT text,EXISTS(SELECT 1 FROM message.migration_invalidations) FROM message.messages WHERE id=$1`, m.message.ID).Scan(&original, &pending); err != nil || original != m.message.Text || !pending {
		t.Fatal("failed original drain lost source or its durable responsibility", err)
	}
	if err = os.Remove(poison); err != nil {
		t.Fatal(err)
	}
	deletionFence(t, owner, func(ctx context.Context) error {
		return owner.freezeMessageContentExportsAt(ctx, m.message.ID, operation, 1)
	})
	bad, err = m.app.readMigration(m.base.ctx, first)
	if err != nil || bad.State != "retired" {
		t.Fatal("fixed original poison could not resume", err)
	}
}

func TestSyncMigrationPGPoisonInterruptedBuildDoesNotStarveNextBatch(t *testing.T) {
	m := newMigrationPG(t)
	_, out := m.create(t)
	template, err := m.app.readMigration(m.base.ctx, out["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(-time.Minute).UTC().Truncate(time.Microsecond)
	var first, last string
	// Interrupted private builders never had a key or an accepted delivery.
	// 129 explicit owned rows exercise a larger set than one worker batch.
	for i := 1; i <= 129; i++ {
		id := fmt.Sprintf("10000000-0000-4000-8000-%012x", i)
		if i == 1 {
			first = id
		}
		last = id
		manifest := template.Manifest
		manifest.ID = id
		manifest.User = newID()
		manifest.Expires = due
		raw, _ := json.Marshal(manifest)
		_, digest, _ := migrationManifestBytes(manifest)
		deletionExec(t, m.base.ctx, m.app, `INSERT INTO sync.migrations(id,scope_id,user_id,request_id,request_digest,source_device,source_store,target_device,target_store,restore_epoch,security_version,source_generation,pairing_id,state,manifest,manifest_digest,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,1,1,1,$10,'building',$11,$12,$13)`, id, m.app.Config.DeploymentID, manifest.User, newID(), make([]byte, 32), template.SourceDevice, template.SourceStore, template.TargetDevice, template.TargetStore, newID(), raw, digest, due)
		path, _ := m.app.migrationPath(id)
		if i == 1 {
			if err = os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
		} else if err = os.WriteFile(path, []byte("private interrupted builder fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		deletionFence(t, m.app, func(ctx context.Context) error {
			if err := m.app.drainMigrationRecords(ctx); err == nil {
				t.Fatal("poison private build falsely completed")
			}
			return nil
		})
	}
	row, err := m.app.readMigration(m.base.ctx, last)
	if err != nil || row.State != "retired" {
		t.Fatal("old poison permanently starved the next bounded batch", err)
	}
	row, err = m.app.readMigration(m.base.ctx, first)
	if err != nil || row.State != "revoking" {
		t.Fatal("poison path was silently accepted", err)
	}
	poison, _ := m.app.migrationPath(first)
	if err = os.Remove(poison); err != nil {
		t.Fatal(err)
	}
	deletionFence(t, m.app, func(ctx context.Context) error { return m.app.drainMigrationRecords(ctx) })
}
