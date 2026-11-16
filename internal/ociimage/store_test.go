package ociimage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("local store must never run rootful")
	}
	s, err := OpenStore(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestStorePutVerifyAndDeduplicate(t *testing.T) {
	s := testStore(t)
	payload := []byte("synthetic non-image bytes for immutable store")
	first, err := s.Put(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if first.Size != int64(len(payload)) || !strings.HasPrefix(first.Digest, "sha256:") {
		t.Fatalf("unexpected descriptor %+v", first)
	}
	size, err := s.Verify(context.Background(), first.Digest)
	if err != nil || size != first.Size {
		t.Fatalf("verify: %v size %d", err, size)
	}
	second, err := s.Put(context.Background(), bytes.NewReader(payload))
	if err != nil || first != second {
		t.Fatalf("dedup: %+v %+v %v", first, second, err)
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "blobs", "sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected blobs: %d", len(entries))
	}
}
func TestStoreRefusesCorruptBlob(t *testing.T) {
	s := testStore(t)
	payload := []byte("immutable test bytes")
	info, err := s.Put(context.Background(), bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.root, "blobs", "sha256", info.Digest[len("sha256:"):])
	if err := os.WriteFile(file, []byte("corrupt contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(context.Background(), info.Digest); err == nil {
		t.Fatal("corrupt blob verified")
	}
	if _, err := s.Put(context.Background(), bytes.NewReader(payload)); err == nil {
		t.Fatal("corrupt existing blob overwritten/accepted")
	}
}
func TestStoreRejectsSymlinkBlob(t *testing.T) {
	s := testStore(t)
	info, err := s.Put(context.Background(), bytes.NewReader([]byte("synthetic")))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.root, "blobs", "sha256", info.Digest[len("sha256:"):])
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Verify(context.Background(), info.Digest); err == nil {
		t.Fatal("symlinked stored blob accepted")
	}
}
func TestStoreRejectsPublicDirectory(t *testing.T) {
	s := testStore(t)
	if err := os.Chmod(s.root, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("public store accepted")
	}
	if _, err := OpenStore(s.root); err == nil {
		t.Fatal("unsafe store reopened")
	}
}
func TestStoreRejectsOversizeAndCleansTemp(t *testing.T) {
	s := testStore(t)
	s.maxBytes = 31
	if _, err := s.Put(context.Background(), bytes.NewReader(bytes.Repeat([]byte("x"), 32))); err == nil {
		t.Fatal("oversize blob accepted")
	}
	files, err := os.ReadDir(filepath.Join(s.root, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatal("temporary file leaked on failure")
	}
}
func TestStoreHonorsCancelledContext(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Put(ctx, bytes.NewReader([]byte("x"))); !errors.Is(err, context.Canceled) {
		t.Fatalf("not cancelled: %v", err)
	}
	if _, err := s.Verify(ctx, "sha256:"+strings.Repeat("a", 64)); err == nil {
		t.Fatal("cancelled verify succeeded")
	}
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }
func TestStoreRejectsNonprogressReader(t *testing.T) {
	s := testStore(t)
	if _, err := s.Put(context.Background(), zeroReader{}); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("stalled reader accepted: %v", err)
	}
}
func TestStoreConcurrentSameDigest(t *testing.T) {
	s := testStore(t)
	payload := bytes.Repeat([]byte("x"), 4096)
	const count = 6
	var wg sync.WaitGroup
	wg.Add(count)
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		go func() {
			defer wg.Done()
			_, err := s.Put(context.Background(), bytes.NewReader(payload))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(filepath.Join(s.root, "blobs", "sha256"))
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected concurrent store state: %v %d", err, len(files))
	}
}
