package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeLockPostAcquisitionInodeGuard(t *testing.T) {
	state := lockFixture(t)
	name := "locks/worker.lock"
	root, err := os.OpenRoot(state)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	f, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyAcquiredContainerLock(root, name, before); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	replacement, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Close()
	if err := verifyAcquiredContainerLock(root, name, before); err == nil {
		t.Fatal("different lock inode trusted after acquiring old file descriptor")
	}
	if err := os.Remove(filepath.Join(state, name)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(state, name)); err != nil {
		t.Fatal(err)
	}
	if err := verifyAcquiredContainerLock(root, name, before); err == nil {
		t.Fatal("symlinked lock pathname accepted after initial opening")
	}
}
func TestRuntimeLockPostAcquisitionNoProductionMutation(t *testing.T) {
	state := lockFixture(t)
	called := 0
	if err := WithContainerLock(context.Background(), state, "worker", func() error { called++; return nil }); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatal("cooperative lock did not run expected synthetic callback")
	}
}
