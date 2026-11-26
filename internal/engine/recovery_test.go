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

type replacingRecoveryRunner struct {
	marker  string
	inPlace bool
}

func (r replacingRecoveryRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	raw, e := os.ReadFile(r.marker)
	if e != nil {
		return nil, e
	}
	if r.inPlace {
		if e = os.WriteFile(r.marker, raw, 0600); e != nil {
			return nil, e
		}
	} else {
		temp := r.marker + ".replacement"
		if e = os.WriteFile(temp, raw, 0600); e != nil {
			return nil, e
		}
		if e = os.Rename(temp, r.marker); e != nil {
			return nil, e
		}
	}
	return []byte(`{"id":"worker","status":"stopped","pid":0}`), nil
}
func TestRecoveryRejectsSameContentMarkerReplacement(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless only")
	}
	for _, inPlace := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "rewrite"}[inPlace], func(t *testing.T) {
			base := t.TempDir()
			if e := os.Chmod(base, 0700); e != nil {
				t.Fatal(e)
			}
			root, e := PrepareStateRoot(base)
			if e != nil {
				t.Fatal(e)
			}
			_, e = RunJournaledMutation(context.Background(), root, "worker", "create", func() ([]byte, error) { return nil, errors.New("synthetic") })
			if e == nil {
				t.Fatal("missing pending marker")
			}
			marker := filepath.Join(root, "journal/worker.pending")
			engine, e := New("/usr/bin/runc", root, replacingRecoveryRunner{marker, inPlace})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = engine.ObserveRecovery(context.Background(), "worker"); e == nil {
				t.Fatal("replaced recovery marker accepted")
			}
			if p, e := ReadPendingMutation(context.Background(), root, "worker"); e != nil || !p.Pending {
				t.Fatal(p, e)
			}
		})
	}
}
