package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalCensusRedactsIDsAndRequiresRecovery(t *testing.T) {
	root := journalFixture(t)
	for _, entry := range []struct{ id, op string }{{"private-a", "create"}, {"private-b", "kill"}} {
		_, err := RunJournaledMutation(context.Background(), root, entry.id, entry.op, func() ([]byte, error) {
			return nil, errors.New("synthetic interrupted runtime")
		})
		if !errors.Is(err, ErrUnresolvedMutation) {
			t.Fatal(err)
		}
	}
	report, err := InspectJournal(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReadOnly || !report.RecoveryRequired || report.RuntimeStateVerified ||
		report.PendingCount != 2 || report.PendingCreate != 1 || report.PendingStop != 1 {
		t.Fatalf("incorrect journal census %+v", report)
	}
	clean := journalFixture(t)
	empty, err := InspectJournal(context.Background(), clean)
	if err != nil || empty.RecoveryRequired || empty.PendingCount != 0 {
		t.Fatalf("empty journal %+v %v", empty, err)
	}
}
func TestJournalCensusRejectsUnsafeOrUnknownEntries(t *testing.T) {
	root := journalFixture(t)
	invalid := filepath.Join(root, "journal", "README")
	if err := os.WriteFile(invalid, []byte("untrusted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectJournal(context.Background(), root); err == nil {
		t.Fatal("unknown file accepted")
	}
	if err := os.Remove(invalid); err != nil {
		t.Fatal(err)
	}
	invalid = filepath.Join(root, "journal", "private.pending")
	if err := os.Symlink("/etc/passwd", invalid); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectJournal(context.Background(), root); err == nil {
		t.Fatal("symlinked marker accepted")
	}
	if err := os.Remove(invalid); err != nil {
		t.Fatal(err)
	}
	invalid = filepath.Join(root, "journal", strings.Repeat("z", 65)+".pending")
	if err := os.WriteFile(invalid, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectJournal(context.Background(), root); err == nil {
		t.Fatal("unsafe ID accepted")
	}
}
func TestJournalCensusRespectsCancellation(t *testing.T) {
	root := journalFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InspectJournal(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled census unexpectedly succeeded")
	}
}
