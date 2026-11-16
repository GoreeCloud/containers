package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func lockFixture(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("locks require rootless user")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}
func TestMutationLockSerializesAndRespectsCancellation(t *testing.T) {
	root := lockFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithContainerLock(context.Background(), root, "alpha", func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()
	err := WithContainerLock(ctx, root, "alpha", func() error { t.Error("overlapping operation ran"); return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent lock did not wait and cancel: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := WithContainerLock(context.Background(), root, "alpha", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}
func TestMutationLockIndependentIDs(t *testing.T) {
	root := lockFixture(t)
	held := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = WithContainerLock(context.Background(), root, "a", func() error { close(held); <-release; return nil })
	}()
	<-held
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := WithContainerLock(ctx, root, "b", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	wg.Wait()
}
func TestMutationLockRejectsUnsafeStateOrSymlink(t *testing.T) {
	root := lockFixture(t)
	if err := WithContainerLock(context.Background(), root, "../bad", func() error { return nil }); err == nil {
		t.Fatal("invalid lock identifier accepted")
	}
	alias := filepath.Join(root, "locks", "alpha.lock")
	if err := os.Symlink("/etc/passwd", alias); err != nil {
		t.Fatal(err)
	}
	if err := WithContainerLock(context.Background(), root, "alpha", func() error { return nil }); err == nil {
		t.Fatal("symlinked lock accepted")
	}
	if err := os.Chmod(filepath.Join(root, "locks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := WithContainerLock(context.Background(), root, "beta", func() error { return nil }); err == nil {
		t.Fatal("public locks directory accepted")
	}
}
func TestMutationLockRejectedOnCanceledContext(t *testing.T) {
	root := lockFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WithContainerLock(ctx, root, "alpha", func() error { t.Error("ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lock accepted: %v", err)
	}
}
