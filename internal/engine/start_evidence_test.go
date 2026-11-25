package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type startEvidenceRunner struct {
	started bool
	post    string
}

func (r *startEvidenceRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch args[2] {
	case "start":
		r.started = true
		return nil, nil
	case "state":
		status := "created"
		pid := 1
		if r.started {
			status = r.post
			if status == "unavailable" {
				return nil, errors.New("synthetic runtime failure")
			}
			if status == "stopped" {
				pid = 0
			}
		}
		return []byte(fmt.Sprintf(`{"id":"worker","status":%q,"pid":%d}`, status, pid)), nil
	default:
		return nil, errors.New("unexpected action")
	}
}
func TestStartEvidenceJournal(t *testing.T) {
	for _, state := range []string{"running", "created", "stopped", "unavailable"} {
		t.Run(state, func(t *testing.T) {
			root := journalFixture(t)
			r := &startEvidenceRunner{post: state}
			e, err := New("/usr/bin/runc", root, r)
			if err != nil {
				t.Fatal(err)
			}
			e.lockMutations = true
			ctx := context.Background()
			err = e.Start(ctx, "worker")
			if state == "running" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnresolvedMutation) {
				t.Fatalf("expected pending incident: %v", err)
			}
			marker, err := ReadPendingMutation(ctx, root, "worker")
			if err != nil || marker.Pending != (state != "running") {
				t.Fatal(marker, err)
			}
		})
	}
}
