package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type stopEvidenceRunner struct {
	stopping bool
	post     string
	reads    int
}

func (r *stopEvidenceRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch args[2] {
	case "kill":
		if args[4] != "SIGTERM" {
			return nil, errors.New("unexpected signal")
		}
		r.stopping = true
		return nil, nil
	case "state":
		s := "running"
		pid := 1
		if r.stopping {
			r.reads++
			if r.post == "unavailable" {
				return nil, errors.New("unavailable")
			}
			if r.post == "created" {
				s = "created"
			}
			if r.post == "stopped" && r.reads > 1 {
				s = "stopped"
				pid = 0
			}
		}
		return []byte(fmt.Sprintf(`{"id":"worker","status":%q,"pid":%d}`, s, pid)), nil
	default:
		return nil, errors.New("unexpected op")
	}
}
func TestStopEvidenceJournal(t *testing.T) {
	for _, post := range []string{"stopped", "created", "unavailable", "running"} {
		t.Run(post, func(t *testing.T) {
			root := journalFixture(t)
			r := &stopEvidenceRunner{post: post}
			e, err := New("/usr/bin/runc", root, r)
			if err != nil {
				t.Fatal(err)
			}
			e.lockMutations = true
			// Positive completion tests should not depend on filesystem fsync
			// and scheduler latency under parallel race-instrumented CI.
			// The production wait remains bounded to three seconds.
			deadline := 10 * time.Second
			if post == "running" {
				deadline = 1500 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			defer cancel()
			err = e.Stop(ctx, "worker")
			if post == "stopped" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, ErrUnresolvedMutation) {
				t.Fatal("expected recovery marker", err)
			}
			p, err := ReadPendingMutation(context.Background(), root, "worker")
			if err != nil || p.Pending != (post != "stopped") {
				t.Fatal(p, err)
			}
		})
	}
}
