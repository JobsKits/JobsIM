package jobsim

import (
	"context"
	"testing"
)

// A bounded worker can stop before the last row. The committed account floor,
// rather than the worker's cursor, must already deny that original body.
func TestAccountDeletionPGUnqueuedLastPageReadDenial(t *testing.T) {
	f := newDeletionPG(t, 0)
	a := f.nodes["message"]
	conversation, other := newID(), newID()
	var last string
	for i := 0; i < 33; i++ {
		id := newID()
		deletionExec(t, f.ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','private unqueued fixture',$6)`, id, conversation, f.user.ID, newID(), []byte("synthetic"), i+1)
		if id > last {
			last = id
		}
	}
	neighbor := newID()
	deletionExec(t, f.ctx, a, `INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES($1,$2,$3,$4,$5,'text','unrelated retained body',34)`, neighbor, conversation, other, newID(), []byte("neighbor"))
	d := f.request(t)
	deletionFence(t, f.nodes["governance"], func(ctx context.Context) error {
		// Stop at the durable pre-page floor, before any lifecycle job exists.
		deletionExec(t, ctx, a, `INSERT INTO message.account_erasure_floors(user_id,deletion_id,request_version) VALUES($1,$2,2)`, f.user.ID, d["deletionId"])
		return nil
	})
	var queued bool
	if err := a.DB.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM message.lifecycle_jobs WHERE message_id=$1)`, last).Scan(&queued); err != nil || queued {
		t.Fatal("fixture did not retain the unqueued last page", err)
	}
	row, err := scanMessage(a.DB.QueryRow(f.ctx, `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, last))
	if err != nil || a.retentionReadState(f.ctx, &row) != nil || !row.Deleted || row.DeleteScope != "global" || row.Text != "" || row.AttachmentID != "" || row.ContentUnavailable {
		t.Fatal("processing account floor exposed an unqueued original body", err)
	}
	if err = a.retentionRequireCloud(f.ctx, last); err == nil {
		t.Fatal("ID-only cloud/export access bypassed the account floor")
	}
	row, err = scanMessage(a.DB.QueryRow(f.ctx, `SELECT `+messageColumns+` FROM message.messages WHERE id=$1`, neighbor))
	if err != nil || a.retentionReadState(f.ctx, &row) != nil || row.Deleted || row.Text != "unrelated retained body" {
		t.Fatal("account floor erased another sender's lawful content", err)
	}
}
