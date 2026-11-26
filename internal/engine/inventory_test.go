package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type inventoryRunner struct{}

func (inventoryRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	if args[2] != "state" {
		return nil, errors.New("unexpected mutation")
	}
	switch args[3] {
	case "worker":
		return []byte(`{"id":"worker","status":"running","pid":42}`), nil
	case "missing":
		return nil, errors.New("synthetic missing")
	}
	return nil, errors.New("synthetic pending")
}
func TestRuntimeInventory(t *testing.T) {
	r := journalFixture(t)
	ctx := context.Background()
	for _, id := range []string{"worker", "missing"} {
		if e := os.Mkdir(filepath.Join(r, id), 0700); e != nil {
			t.Fatal(e)
		}
	}
	_, e := RunJournaledMutation(ctx, r, "pending", "create", func() ([]byte, error) { return nil, errors.New("synthetic") })
	if e == nil {
		t.Fatal("expected pending")
	}
	engine, e := New("/usr/bin/runc", r, inventoryRunner{})
	if e != nil {
		t.Fatal(e)
	}
	engine.lockMutations = true
	p, e := engine.Inventory(ctx)
	if e != nil || len(p.Entries) != 3 || p.Running != 1 || p.Uncertain != 2 || !p.RecoveryRequired || p.AtomicSnapshot {
		t.Fatal(p, e)
	}
	if p.Entries[1].ID != "pending" || p.Entries[1].MetadataPresent || p.Entries[1].PendingOperation != "create" {
		t.Fatal(p)
	}
	if e = os.Symlink("worker", filepath.Join(r, "alias")); e != nil {
		t.Fatal(e)
	}
	if _, e = engine.Inventory(ctx); e == nil {
		t.Fatal("symlink inventory accepted")
	}
}
