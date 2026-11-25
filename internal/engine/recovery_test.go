package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type recoveryRunner struct{ fail bool }

func (r recoveryRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if r.fail {
		return nil, errors.New("runtime unavailable")
	}
	return []byte(`{"id":"worker","status":"stopped","pid":0}`), nil
}
func TestRecoveryObservationPreservesMarker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless only")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RunJournaledMutation(context.Background(), root, "worker", "create", func() ([]byte, error) { return nil, errors.New("synthetic interruption") })
	if err == nil {
		t.Fatal("expected interruption")
	}
	for _, fail := range []bool{false, true} {
		e, err := New("/usr/bin/runc", root, recoveryRunner{fail})
		if err != nil {
			t.Fatal(err)
		}
		report, err := e.ObserveRecovery(context.Background(), "worker")
		if err != nil {
			t.Fatal(err)
		}
		if !report.Pending.Pending || report.RecoveryAuthorized || (report.Observed == nil) != fail {
			t.Fatalf("unexpected report %+v", report)
		}
		if _, err := os.Stat(filepath.Join(root, "journal/worker.pending")); err != nil {
			t.Fatal(err)
		}
	}
}
