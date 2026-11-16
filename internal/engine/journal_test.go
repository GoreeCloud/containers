package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func journalFixture(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("journal requires rootless user")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	return root
}
func TestJournalSuccessAndReadOnlyStatus(t *testing.T) {
	root := journalFixture(t)
	count := 0
	result, err := RunJournaledMutation(context.Background(), root, "worker", "start", func() ([]byte, error) {
		count++
		pending, err := ReadPendingMutation(context.Background(), root, "worker")
		if err != nil || !pending.Pending || !pending.RecoveryRequired || pending.Operation != "start" {
			t.Fatalf("intent marker absent when operation executed: %+v %v", pending, err)
		}
		return []byte("ok"), nil
	})
	if err != nil || string(result) != "ok" || count != 1 {
		t.Fatalf("journal success: %q %v", result, err)
	}
	status, err := ReadPendingMutation(context.Background(), root, "worker")
	if err != nil || status.Pending || status.RecoveryRequired || !status.ReadOnly {
		t.Fatalf("completed marker not cleared %+v %v", status, err)
	}
}
func TestJournalFailClosedAfterUncertainRuntimeFailure(t *testing.T) {
	root := journalFixture(t)
	_, err := RunJournaledMutation(context.Background(), root, "worker", "create", func() ([]byte, error) {
		return nil, errors.New("synthetic OCI runtime failure")
	})
	if !errors.Is(err, ErrUnresolvedMutation) {
		t.Fatalf("uncertain OCI error misclassified: %v", err)
	}
	status, err := ReadPendingMutation(context.Background(), root, "worker")
	if err != nil || !status.Pending || !status.RecoveryRequired {
		t.Fatalf("pending state not recorded: %+v %v", status, err)
	}
	ran := false
	_, err = RunJournaledMutation(context.Background(), root, "worker", "delete", func() ([]byte, error) { ran = true; return nil, nil })
	if !errors.Is(err, ErrUnresolvedMutation) || ran {
		t.Fatal("second mutation ran with unresolved prior operation")
	}
}
func TestJournalRejectsMalformedOrSymlinkedMarkers(t *testing.T) {
	root := journalFixture(t)
	name := filepath.Join(root, "journal", "worker.pending")
	if err := os.Symlink("/etc/passwd", name); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPendingMutation(context.Background(), root, "worker"); err == nil {
		t.Fatal("symlinked pending file accepted")
	}
	if _, err := RunJournaledMutation(context.Background(), root, "worker", "start", func() ([]byte, error) { t.Fatal("unexpected runtime call"); return nil, nil }); err == nil {
		t.Fatal("symlinked marker accepted")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(`{"version":1,"id":"worker","id":"other","operation":"create"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPendingMutation(context.Background(), root, "worker"); err == nil {
		t.Fatal("duplicate-key journal accepted")
	}
}
func TestJournalValidatesInputAndCancellation(t *testing.T) {
	root := journalFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunJournaledMutation(ctx, root, "worker", "start", func() ([]byte, error) { t.Fatal("unexpected"); return nil, nil }); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled mutation accepted")
	}
	if _, err := RunJournaledMutation(context.Background(), root, "../escape", "create", func() ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("invalid id accepted")
	}
	if _, err := RunJournaledMutation(context.Background(), root, "worker", "state", func() ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("read-only state incorrectly accepted as mutation")
	}
	if _, err := ReadPendingMutation(context.Background(), root, "worker"); err != nil {
		t.Fatal(err)
	}
}
