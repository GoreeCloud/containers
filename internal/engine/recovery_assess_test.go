package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessmentMatrix(t *testing.T) {
	for _, op := range []string{"create", "start", "kill", "delete"} {
		for _, status := range []string{"created", "running", "stopped", "unavailable"} {
			o := RecoveryObservation{Pending: PendingMutation{ID: "worker", Operation: op, Pending: true, ReadOnly: true, RecoveryRequired: true}}
			if status != "unavailable" {
				pid := int64(123)
				if status == "stopped" {
					pid = 0
				}
				o.Observed = &RuntimeState{ID: "worker", Status: status, PID: pid}
			}
			r, e := assessRecovery(o)
			want := op == "create" && status == "created" || op == "start" && status == "running" || op == "kill" && status == "stopped"
			if e != nil || r.PostconditionObserved != want || !r.RecoveryRequired || r.JournalCleared || r.RecoveryAuthorized || r.IdentityVerified || !strings.HasPrefix(r.ObservationSHA256, "sha256:") {
				t.Fatal(r, e)
			}
		}
	}
}
func TestAssessmentPreservesMarkerAndPin(t *testing.T) {
	root := journalFixture(t)
	ctx := context.Background()
	_, e := RunJournaledMutation(ctx, root, "worker", "kill", func() ([]byte, error) { return nil, errors.New("interruption") })
	if e == nil {
		t.Fatal("expected incident")
	}
	runtime, e := New("/usr/bin/runc", root, recoveryRunner{})
	if e != nil {
		t.Fatal(e)
	}
	r, e := runtime.AssessRecovery(ctx, "worker", "")
	if e != nil || !r.PostconditionObserved {
		t.Fatal(r, e)
	}
	again, e := runtime.AssessRecovery(ctx, "worker", r.ObservationSHA256)
	if e != nil || again != r {
		t.Fatal(again, e)
	}
	if _, e = runtime.AssessRecovery(ctx, "worker", "sha256:"+strings.Repeat("0", 64)); e == nil {
		t.Fatal("wrong pin accepted")
	}
	if _, e = os.Stat(filepath.Join(root, "journal", "worker.pending")); e != nil {
		t.Fatal(e)
	}
}
