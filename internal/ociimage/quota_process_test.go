package ociimage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This test actually starts a separate synthetic Go test process. It never
// starts a container or runs an external OCI runtime.
func TestCrossProcessContentQuotaLock(t *testing.T) {
	store := testStore(t)
	root, err := store.openRoot()
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := store.acquireWriter(context.Background(), root)
	if err != nil {
		root.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestContentQuotaLockHelper$")
	child.Env = []string{"GC_QUOTA_CHILD=1", "GC_QUOTA_STORE=" + store.root}
	output, err := child.CombinedOutput()
	unlock()
	root.Close()
	if err != nil {
		t.Fatalf("second process failed: %v: %s", err, output)
	}
	if !strings.Contains(string(output), "WRITER_LOCK_BLOCKED") {
		t.Fatalf("second process did not observe private cross-process quota lock: %s", output)
	}
	report, err := store.Audit(context.Background())
	if err != nil || !report.Verified || report.BlobCount != 0 {
		t.Fatalf("synthetic lock test changed store: %+v %v", report, err)
	}
}
func TestContentQuotaLockHelper(t *testing.T) {
	if os.Getenv("GC_QUOTA_CHILD") != "1" {
		return
	}
	store, err := OpenExistingStore(os.Getenv("GC_QUOTA_STORE"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 160*time.Millisecond)
	defer cancel()
	_, err = store.Put(ctx, bytes.NewReader([]byte("synthetic opaque bytes")))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent import bypassed cross-process writer lock: %v", err)
	}
	fmt.Println("WRITER_LOCK_BLOCKED")
}

// Renaming and replacing the lock path during acquisition must not make a
// cooperating writer trust a stale inode. It cannot defend against a same-user
// attacker replacing it after admission; root-level hostile controls remain
// a separate unsatisfied release gate.
func TestQuotaLockRejectsReplacementAfterAcquisition(t *testing.T) {
	store := testStore(t)
	root, err := store.openRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	release, err := store.acquireWriter(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	before, err := root.Lstat(".store-write.lock")
	if err != nil {
		t.Fatal(err)
	}
	if !before.Mode().IsRegular() {
		t.Fatal("lock not a regular file")
	}
	// Best-effort post-lock consistency is tested by replacing the path
	// before the next lock attempt. The foreign inode must be checked.
	if err := root.Remove(".store-write.lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", store.root+"/.store-write.lock"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("replaced quota lock symlink allowed")
	}
}
