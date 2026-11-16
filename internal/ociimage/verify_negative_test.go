package ociimage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRejectsUnsafeCommittedBlobMetadata(t *testing.T) {
	store := testStore(t)
	item, err := store.Put(context.Background(), bytes.NewReader([]byte("private synthetic test bytes")))
	if err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(store.root, "blobs", "sha256", strings.TrimPrefix(item.Digest, "sha256:"))
	if err := os.Chmod(blob, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(context.Background(), item.Digest); err == nil {
		t.Fatal("group-readable committed blob unexpectedly passed verification")
	}
	if err := os.Chmod(blob, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(store.root, "linked-outside")
	if err := os.Link(blob, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(context.Background(), item.Digest); err == nil {
		t.Fatal("hardlinked committed blob unexpectedly passed verification")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if size, err := store.Verify(context.Background(), item.Digest); err != nil || size != item.Size {
		t.Fatalf("safe committed blob verification failed: size=%d error=%v", size, err)
	}
	if err := os.Remove(blob); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", blob); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Verify(context.Background(), item.Digest); err == nil {
		t.Fatal("symlinked blob unexpectedly passed verification")
	}
}
func TestVerifyRejectsInvalidContextAndDigest(t *testing.T) {
	store := testStore(t)
	if _, err := store.Verify(context.Background(), "invalid"); err == nil {
		t.Fatal("malformed digest accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Verify(ctx, "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("cancelled verification accepted")
	}
}
