package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recoverySummaryRunner struct{}

func (recoverySummaryRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	id := args[len(args)-1]
	if id == "unavailable-private" {
		return nil, errors.New("synthetic")
	}
	state, pid := "stopped", 0
	if id == "started-private" {
		state, pid = "running", 101
	}
	raw, _ := json.Marshal(RuntimeState{ID: id, Status: state, PID: int64(pid)})
	return raw, nil
}
func TestRecoverySummaryPrivacyAndPreservation(t *testing.T) {
	root := journalFixture(t)
	ctx := context.Background()
	for id, op := range map[string]string{"started-private": "start", "stopped-private": "kill", "delete-private": "delete", "unavailable-private": "create", "conflict-private": "create"} {
		if _, e := RunJournaledMutation(ctx, root, id, op, func() ([]byte, error) { return nil, errors.New("synthetic interruption") }); e == nil {
			t.Fatal("expected pending")
		}
	}
	rt, e := New("/usr/bin/runc", root, recoverySummaryRunner{})
	if e != nil {
		t.Fatal(e)
	}
	r, e := rt.SummarizeRecovery(ctx)
	if e != nil || r.PendingCount != 5 || r.PostconditionObserved != 2 || r.DeleteReview != 1 || r.RuntimeUnavailable != 1 || r.StateConflict != 1 || !r.RecoveryRequired || r.AtomicSnapshot || r.RecoveryAuthorized {
		t.Fatal(r, e)
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "private") {
		t.Fatal("ID leaked")
	}
	for _, id := range []string{"started-private", "stopped-private", "delete-private", "unavailable-private", "conflict-private"} {
		if _, e = os.Stat(filepath.Join(root, "journal", id+".pending")); e != nil {
			t.Fatal(e)
		}
	}
}
