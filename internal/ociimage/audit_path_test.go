package ociimage

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestAuditedBlobPathDetectsUnlinkAndReplacement(t *testing.T) {
	store := testStore(t)
	item, err := store.Put(context.Background(), bytes.NewReader([]byte("synthetic OCI payload")))
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.openRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := "blobs/sha256/" + strings.TrimPrefix(item.Digest, "sha256:")
	prior, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := confirmAuditedBlobPath(root, f, prior, name); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	replacement, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replacement.Write([]byte("synthetic OCI payload")); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := confirmAuditedBlobPath(root, f, prior, name); err == nil {
		t.Fatal("same-size replacement path passed post-read identity check")
	}
}
func TestAuditedBlobPathDetectsChangedPermissions(t *testing.T) {
	store := testStore(t)
	item, err := store.Put(context.Background(), bytes.NewReader([]byte("synthetic blob")))
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.openRoot()
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	name := "blobs/sha256/" + strings.TrimPrefix(item.Digest, "sha256:")
	prior, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := root.Chmod(name, 0644); err != nil {
		t.Fatal(err)
	}
	if err := confirmAuditedBlobPath(root, f, prior, name); err == nil {
		t.Fatal("world-readable in-place blob mode change passed audit")
	}
}
