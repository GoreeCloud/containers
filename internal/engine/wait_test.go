package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

type waitRunner struct {
	states []string
	reads  int
}

func (r *waitRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if len(args) < 3 || args[2] != "state" {
		return nil, errors.New("unexpected mutation")
	}
	i := min(r.reads, len(r.states)-1)
	r.reads++
	return []byte(r.states[i]), nil
}
func TestWaitStoppedReadOnly(t *testing.T) {
	running := `{"id":"worker","status":"running","pid":42}`
	stopped := `{"id":"worker","status":"stopped","pid":0}`
	for _, tc := range []struct {
		name   string
		states []string
		ok     bool
	}{{"stop", []string{running, stopped}, true}, {"already stopped", []string{stopped}, true}, {"pid changed", []string{running, `{"id":"worker","status":"running","pid":43}`}, false}, {"created", []string{`{"id":"worker","status":"created","pid":42}`}, false}, {"bad stopped", []string{`{"id":"worker","status":"stopped","pid":42}`}, false}, {"deadline", []string{running}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			r := &waitRunner{states: tc.states}
			e, err := New("/usr/bin/runc", journalFixture(t), r)
			if err != nil {
				t.Fatal(err)
			}
			e.lockMutations = true
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			report, err := e.WaitStopped(ctx, "worker", time.Second)
			if tc.ok {
				if err != nil || !report.Stopped || !report.ReadOnly || report.ExitCodeVerified || report.InstanceIdentityVerified {
					t.Fatal(report, err)
				}
			} else if err == nil {
				t.Fatal("unexpected wait success", report)
			}
		})
	}
}
func TestWaitRefusesPending(t *testing.T) {
	root := journalFixture(t)
	_, err := RunJournaledMutation(context.Background(), root, "worker", "start", func() ([]byte, error) { return nil, errors.New("synthetic") })
	if err == nil {
		t.Fatal("expected pending")
	}
	r := &waitRunner{states: []string{`{"id":"worker","status":"stopped","pid":0}`}}
	e, err := New("/usr/bin/runc", root, r)
	if err != nil {
		t.Fatal(err)
	}
	e.lockMutations = true
	if _, err = e.WaitStopped(context.Background(), "worker", time.Second); !errors.Is(err, ErrUnresolvedMutation) || r.reads != 0 {
		t.Fatal(err, r.reads)
	}
}
