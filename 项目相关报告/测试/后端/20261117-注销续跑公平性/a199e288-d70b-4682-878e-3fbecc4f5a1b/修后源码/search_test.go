package jobsim

import (
	"testing"
	"time"
)

func TestMessageListCursorBindsOwnerEpochAndQuery(t *testing.T) {
	a := &App{Config: Config{Key: []byte("isolated-cursor-key")}}
	u := User{ID: newID(), RestoreEpoch: "9007199254740993"}
	v := messageListCursor{Owner: u.ID, Epoch: u.RestoreEpoch, Filter: "query-and-mode", Created: time.Now().UTC(), ID: newID()}
	raw := a.encodeMessageCursor(v)
	read, e := a.decodeMessageCursor(raw, u, v.Filter)
	if e != nil || read.ID != v.ID || read.Epoch != v.Epoch {
		t.Fatal("exact cursor did not round trip")
	}
	if _, e = a.decodeMessageCursor(raw+"x", u, v.Filter); e == nil {
		t.Fatal("tampered cursor accepted")
	}
	other := u
	other.ID = newID()
	if _, e = a.decodeMessageCursor(raw, other, v.Filter); e == nil {
		t.Fatal("other owner borrowed cursor")
	}
	if _, e = a.decodeMessageCursor(raw, u, "different-query"); e == nil {
		t.Fatal("cursor query changed")
	}
	u.RestoreEpoch = "9007199254740994"
	_, e = a.decodeMessageCursor(raw, u, v.Filter)
	failure, ok := e.(*apiError)
	if !ok || failure.Code != "RESTORE_EPOCH_CHANGED" {
		t.Fatal("old restore cursor accepted")
	}
}
