package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalRetainsReplacementAndCancellation(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		root := journalFixture(t)
		name := filepath.Join(root, "journal", "worker.pending")
		_, err := RunJournaledMutation(context.Background(), root, "worker", "start", func() ([]byte, error) {
			if err := os.Rename(name, name+".retained"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte(`{"version":1,"id":"worker","operation":"delete"}`), 0600); err != nil {
				t.Fatal(err)
			}
			return []byte("ok"), nil
		})
		if !errors.Is(err, ErrUnresolvedMutation) {
			t.Fatalf("unexpected result: %v", err)
		}
		pending, err := ReadPendingMutation(context.Background(), root, "worker")
		if err != nil || !pending.Pending || pending.Operation != "delete" {
			t.Fatalf("replacement lost: %+v %v", pending, err)
		}
	})
	t.Run("cancelled after action", func(t *testing.T) {
		root := journalFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, err := RunJournaledMutation(ctx, root, "worker", "start", func() ([]byte, error) { cancel(); return []byte("ok"), nil })
		if !errors.Is(err, ErrUnresolvedMutation) {
			t.Fatalf("unexpected result: %v", err)
		}
		pending, err := ReadPendingMutation(context.Background(), root, "worker")
		if err != nil || !pending.Pending {
			t.Fatalf("marker lost: %+v %v", pending, err)
		}
	})
}
