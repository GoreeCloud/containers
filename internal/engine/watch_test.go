package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchStopsAtIncident(t *testing.T) {
	root := journalFixture(t)
	ctx := context.Background()
	e, err := New("/usr/bin/runc", root, inventoryRunner{})
	if err != nil {
		t.Fatal(err)
	}
	e.lockMutations = true
	if err = os.Mkdir(filepath.Join(root, "worker"), 0700); err != nil {
		t.Fatal(err)
	}
	count := 0
	err = e.Watch(ctx, 3, time.Second, func(s WatchSample) error {
		count++
		if s.Sequence != count || !s.ReadOnly || s.AutoRepair || s.AtomicSnapshot {
			t.Fatal(s)
		}
		if count == 1 {
			if s.Running != 1 || s.RecoveryRequired {
				t.Fatal(s)
			}
			_, err := RunJournaledMutation(ctx, root, "pending", "create", func() ([]byte, error) { return nil, errors.New("synthetic") })
			if err == nil {
				t.Fatal("expected incident")
			}
		}
		if count == 2 && (!s.RecoveryRequired || s.Uncertain != 1 || s.Running != 1) {
			t.Fatal(s)
		}
		return nil
	})
	if !errors.Is(err, ErrUnresolvedMutation) || count != 2 {
		t.Fatal(count, err)
	}
	p, err := ReadPendingMutation(ctx, root, "pending")
	if err != nil || !p.Pending {
		t.Fatal(p, err)
	}
}
func TestWatchCancellationAndBounds(t *testing.T) {
	root := journalFixture(t)
	e, err := New("/usr/bin/runc", root, inventoryRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	err = e.Watch(ctx, 2, time.Second, func(s WatchSample) error { count++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || count != 1 {
		t.Fatal(count, err)
	}
	if err = e.Watch(context.Background(), 61, time.Second, func(WatchSample) error { return nil }); err == nil {
		t.Fatal("bad bound")
	}
}
