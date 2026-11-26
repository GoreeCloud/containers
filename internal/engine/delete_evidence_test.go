package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type deleteEvidenceRunner struct {
	root string
	mode string
}

func (r deleteEvidenceRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	switch args[2] {
	case "state":
		return []byte(`{"id":"worker","status":"stopped","pid":0}`), nil
	case "delete":
		p := filepath.Join(r.root, "worker")
		switch r.mode {
		case "absent":
			return nil, os.Remove(p)
		case "symlink":
			if e := os.Remove(p); e != nil {
				return nil, e
			}
			return nil, os.Symlink("journal", p)
		case "failure":
			return nil, errors.New("synthetic")
		}
		return nil, nil
	default:
		return nil, errors.New("unexpected runtime call")
	}
}
func TestDeleteRequiresVerifiedMetadataAbsence(t *testing.T) {
	for _, mode := range []string{"absent", "retained", "symlink", "failure", "missing-before"} {
		t.Run(mode, func(t *testing.T) {
			root := journalFixture(t)
			if mode != "missing-before" {
				if e := os.Mkdir(filepath.Join(root, "worker"), 0700); e != nil {
					t.Fatal(e)
				}
			}
			engine, e := New("/usr/bin/runc", root, deleteEvidenceRunner{root, mode})
			if e != nil {
				t.Fatal(e)
			}
			engine.lockMutations = true
			e = engine.Delete(context.Background(), "worker")
			if mode == "absent" {
				if e != nil {
					t.Fatal(e)
				}
			} else if !errors.Is(e, ErrUnresolvedMutation) {
				t.Fatal("missing incident", e)
			}
			pending, e := ReadPendingMutation(context.Background(), root, "worker")
			if e != nil || pending.Pending != (mode != "absent") {
				t.Fatal(pending, e)
			}
		})
	}
}
func TestReservedRuntimeIDs(t *testing.T) {
	for _, id := range []string{"locks", "journal"} {
		if ValidateID(id) == nil {
			t.Fatal("reserved metadata ID accepted")
		}
	}
}
