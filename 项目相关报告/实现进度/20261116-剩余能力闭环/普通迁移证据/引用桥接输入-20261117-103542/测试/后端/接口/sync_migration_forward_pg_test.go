package jobsim

import (
	"context"
	"os"
	"testing"
)

func TestSyncMigrationPGParentFreezeDrainsDerivedForwardPackage(t *testing.T) {
	m := newMigrationPG(t)
	parent := m.message
	id := newID()
	owner := m.base.nodes["message"]
	deletionExec(t, m.base.ctx, owner, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'forward','',2)`, id, parent.ConversationID, m.base.user.ID, newID(), []byte("explicit original reference fixture"))
	deletionExec(t, m.base.ctx, owner, `INSERT INTO message.forward_refs(message_id,source_message_id,source_conversation_id,source_content_version,forwarder_id,source_membership_generation,security_version,restore_epoch,depth) VALUES($1,$2,$3,1,$4,1,1,1,1)`, id, parent.ID, parent.ConversationID, m.base.user.ID)
	var err error
	m.message, err = owner.loadMessage(m.base.ctx, id, m.base.user.ID)
	if err != nil || !m.message.Forwarded || m.message.ForwardUnavailable || m.message.Text != parent.Text {
		t.Fatal("production current forward projection unavailable", err)
	}
	_, out := m.create(t)
	transfer := out["id"].(string)
	deletionFence(t, owner, func(ctx context.Context) error {
		return owner.freezeMessageContentExportsAt(ctx, parent.ID, "original-forward-parent:"+newID(), 1)
	})
	rec, err := m.app.readMigration(m.base.ctx, transfer)
	if err != nil || rec.State != "retired" || len(rec.Key) != 0 {
		t.Fatal("parent freeze returned while derived migration retained delivery", err)
	}
	path, _ := m.app.migrationPath(transfer)
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("derived current-reference capsule was not drained", err)
	}
	current, err := owner.loadMessage(m.base.ctx, id, m.base.user.ID)
	if err != nil || !current.ForwardUnavailable || current.Text != "" {
		t.Fatal("retired reference retained its source projection", err)
	}
	var body string
	if err = owner.DB.QueryRow(m.base.ctx, `SELECT text FROM message.messages WHERE id=$1`, parent.ID).Scan(&body); err != nil || body != parent.Text {
		t.Fatal("source content disappeared before its enclosing original mutation", err)
	}
}
