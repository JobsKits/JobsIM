package jobsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The first two original responsibilities have persistent per-account transport
// failures. A third valid responsibility must still receive a bounded attempt.
// All role handlers and SQL are real; only those two failures are injected.
type closureAuditDeletionFailures struct {
	base    http.RoundTripper
	path    string
	blocked map[string]bool
}

func (f closureAuditDeletionFailures) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path == f.path {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var d accountDeletion
		if json.Unmarshal(raw, &d) != nil {
			return nil, errors.New("invalid private deletion fixture request")
		}
		if f.blocked[d.ID] {
			return nil, errors.New("persistent failure for one original account")
		}
	}
	return f.base.RoundTrip(r)
}

func closureAuditPreparedDeletions(t *testing.T, f *deletionPGFixture, governance bool) []accountDeletion {
	t.Helper()
	items := []accountDeletion{}
	for i := 0; i < 3; i++ {
		d := accountDeletion{ID: newID(), User: newID(), Request: newID(), Device: newID(), Epoch: "1", State: "prepared", Version: "1", Expected: 1, Security: 2, Until: time.Now().UTC().Add(7 * 24 * time.Hour).Truncate(time.Microsecond)}
		name := "fair_" + strings.ReplaceAll(d.User, "-", "")[:20]
		deletionExec(t, f.ctx, f.nodes["identity"], `INSERT INTO auth.users(id,username,display_name,password_hash,pending_operation_id) VALUES($1,$2,'Non-secret account fixture',$3,$4)`, d.User, name, pcHash(t, f.ctx, f.nodes["identity"], f.user.ID), deletionOperation(d, "request"))
		deletionExec(t, f.ctx, f.nodes["identity"], `INSERT INTO auth.account_deletions(id,user_id,request_id,device_id,restore_epoch,request_digest,state,expected_security,security_version,cancel_until,updated_at)
VALUES($1,$2,$3,$4,1,$5,'prepared',1,2,$6,clock_timestamp()-interval '1 hour'+$7*interval '1 second')`, d.ID, d.User, d.Request, d.Device, tokenDigest("non-secret original proof "+d.ID), d.Until, i)
		if governance {
			deletionExec(t, f.ctx, f.nodes["governance"], `INSERT INTO governance.account_deletions(id,user_id,request_id,device_id,restore_epoch,state,cancel_until,updated_at)
VALUES($1,$2,$3,$4,1,'prepared',$5,clock_timestamp()-interval '1 hour'+$6*interval '1 second')`, d.ID, d.User, d.Request, d.Device, d.Until, i)
		}
		items = append(items, d)
	}
	return items
}

func closureAuditRunDeletionRecovery(t *testing.T, ctx context.Context, a *App) {
	t.Helper()
	// Failed original responsibilities remain pending. The test asserts progress
	// separately instead of treating one batch's expected error as completion.
	deletionFence(t, a, func(held context.Context) error {
		_ = a.resumeAccountDeletions(held, 2)
		return nil
	})
}

func closureAuditCheckPrepared(t *testing.T, f *deletionPGFixture, items []accountDeletion, governance bool) {
	t.Helper()
	for _, d := range items[:2] {
		var state, pending string
		var security int64
		var until time.Time
		if err := f.nodes["identity"].DB.QueryRow(f.ctx, `SELECT d.state,d.cancel_until,u.pending_operation_id,u.security_version FROM auth.account_deletions d JOIN auth.users u ON u.id=d.user_id WHERE d.id=$1`, d.ID).Scan(&state, &until, &pending, &security); err != nil {
			t.Fatal("original pending responsibility unavailable")
		}
		if state != "prepared" || !until.Equal(d.Until) || pending != deletionOperation(d, "request") || security != 1 {
			t.Fatal("failed original was discarded, rescheduled or changed credential state")
		}
		if governance {
			if err := f.nodes["governance"].DB.QueryRow(f.ctx, `SELECT state,cancel_until FROM governance.account_deletions WHERE id=$1`, d.ID).Scan(&state, &until); err != nil || state != "prepared" || !until.Equal(d.Until) {
				t.Fatal("failed governance responsibility lost its original state or deadline")
			}
		}
	}
}

func TestClosureAuditDeletionIdentityRecoveryDoesNotStarveLaterIntent(t *testing.T) {
	f := newDeletionPG(t, 7)
	items := closureAuditPreparedDeletions(t, f, false)
	a := f.nodes["identity"]
	a.Client = &http.Client{Transport: closureAuditDeletionFailures{f.wire, "/internal/deletion/prepare", map[string]bool{items[0].ID: true, items[1].ID: true}}}
	for range 3 {
		closureAuditRunDeletionRecovery(t, f.ctx, a)
	}
	closureAuditCheckPrepared(t, f, items, false)
	var state string
	if err := a.DB.QueryRow(f.ctx, `SELECT state FROM auth.account_deletions WHERE id=$1`, items[2].ID).Scan(&state); err != nil || state != "requested" {
		t.Fatal("two persistent original failures starved a later valid identity request")
	}
}

func TestClosureAuditDeletionGovernanceRecoveryDoesNotStarveLaterIntent(t *testing.T) {
	f := newDeletionPG(t, 7)
	items := closureAuditPreparedDeletions(t, f, true)
	a := f.nodes["governance"]
	a.Client = &http.Client{Transport: closureAuditDeletionFailures{f.wire, "/internal/deletion/activate", map[string]bool{items[0].ID: true, items[1].ID: true}}}
	for range 3 {
		closureAuditRunDeletionRecovery(t, f.ctx, a)
	}
	closureAuditCheckPrepared(t, f, items, true)
	var state string
	if err := a.DB.QueryRow(f.ctx, `SELECT state FROM governance.account_deletions WHERE id=$1`, items[2].ID).Scan(&state); err != nil || state != "requested" {
		t.Fatal("two persistent original failures starved a later valid governance request")
	}
}
