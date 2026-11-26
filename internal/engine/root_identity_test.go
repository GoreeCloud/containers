package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLockDetectsRootAndControlDirectoryReplacement(t *testing.T) {
	for _, sub := range []string{".", "locks"} {
		t.Run(sub, func(t *testing.T) {
			root := lockFixture(t)
			path := filepath.Join(root, sub)
			old := path + ".previous"
			e := WithContainerLock(context.Background(), root, "alpha", func() error {
				if e := os.Rename(path, old); e != nil {
					return e
				}
				return os.Mkdir(path, 0700)
			})
			if e == nil {
				t.Fatal("replacement lock root reported success")
			}
		})
	}
}
func TestJournalKeepsEvidenceWhenControlRootReplaced(t *testing.T) {
	for _, sub := range []string{".", "journal"} {
		t.Run(sub, func(t *testing.T) {
			root := journalFixture(t)
			path := filepath.Join(root, sub)
			old := path + ".previous"
			_, e := RunJournaledMutation(context.Background(), root, "worker", "create", func() ([]byte, error) {
				if e := os.Rename(path, old); e != nil {
					return nil, e
				}
				if e := os.Mkdir(path, 0700); e != nil {
					return nil, e
				}
				return []byte("synthetic success"), nil
			})
			if !errors.Is(e, ErrUnresolvedMutation) {
				t.Fatal(e)
			}
			preserved := filepath.Join(old, "worker.pending")
			if sub == "." {
				preserved = filepath.Join(old, "journal", "worker.pending")
			}
			if _, e = os.Stat(preserved); e != nil {
				t.Fatal("original pending evidence missing", e)
			}
		})
	}
}
func TestRejectSymlinkedRuntimeParent(t *testing.T) {
	root := lockFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if e := os.Symlink(filepath.Dir(root), alias); e != nil {
		t.Fatal(e)
	}
	name := filepath.Join(alias, filepath.Base(root))
	if e := WithContainerLock(context.Background(), name, "alpha", func() error { t.Fatal("unexpected callback"); return nil }); e == nil {
		t.Fatal("symlink parent accepted")
	}
	if _, e := ReadPendingMutation(context.Background(), name, "alpha"); e == nil {
		t.Fatal("symlink journal parent accepted")
	}
}
