package engine

import (
	"context"
	"errors"
	"testing"
)

func TestEveryLifecycleOperationRefusesUnresolvedJournal(t *testing.T) {
	root := journalFixture(t)
	f := &fakeRunner{status: "running"}
	e, err := New("/usr/bin/runc", root, f)
	if err != nil {
		t.Fatal(err)
	}
	e.lockMutations = true
	ctx := context.Background()
	_, err = RunJournaledMutation(ctx, root, "worker", "create", func() ([]byte, error) { return nil, errors.New("synthetic interruption") })
	if !errors.Is(err, ErrUnresolvedMutation) {
		t.Fatal(err)
	}
	for name, op := range map[string]func() error{
		"create": func() error { return e.Create(ctx, "worker", "/nonexistent/fixture") },
		"start":  func() error { return e.Start(ctx, "worker") },
		"stop":   func() error { return e.Stop(ctx, "worker") },
		"delete": func() error { return e.Delete(ctx, "worker") },
		"state":  func() error { _, err := e.State(ctx, "worker"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := op(); !errors.Is(err, ErrUnresolvedMutation) {
				t.Fatalf("expected recovery gate, got %v", err)
			}
			if len(f.argv) != 0 {
				t.Fatal("runtime was called despite unresolved journal")
			}
		})
	}
}
