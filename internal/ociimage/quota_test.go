package ociimage

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAggregateQuotaBlocksNewUniqueBlobButAllowsDedup(t *testing.T) {
	store := testStore(t)
	store.maxTotalBytes = 22
	data := []byte("1234567890") // 10
	first, err := store.Put(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader([]byte("abcdefghij"))); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), bytes.NewReader([]byte("abc"))); err == nil {
		t.Fatal("aggregate quota exceeded")
	}
	repeated, err := store.Put(context.Background(), bytes.NewReader(data))
	if err != nil || first != repeated {
		t.Fatalf("dedup under near-full quota failed: %+v %v", repeated, err)
	}
	report, err := store.Audit(context.Background())
	if err != nil || report.TotalBytes != 20 || report.BlobCount != 2 {
		t.Fatalf("store mutated after quota denial: %+v %v", report, err)
	}
}
func TestAggregateStoreQuotaConcurrentUniqueBlobs(t *testing.T) {
	s := testStore(t)
	s.maxTotalBytes = 50
	const workers = 7
	var wg sync.WaitGroup
	result := make(chan error, workers)
	for i := 0; i < workers; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Put(context.Background(), bytes.NewReader([]byte(strings.Repeat(string(rune('A'+i)), 10))))
			result <- err
		}()
	}
	wg.Wait()
	close(result)
	succeeded := 0
	failed := 0
	for err := range result {
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	if succeeded != 5 || failed != 2 {
		t.Fatalf("concurrent quota enforcement: succeeded=%d failed=%d", succeeded, failed)
	}
	report, err := s.Audit(context.Background())
	if err != nil || report.TotalBytes != 50 || report.BlobCount != 5 {
		t.Fatalf("unexpected final committed bytes: %+v %v", report, err)
	}
}
func TestStoreQuotaLockSymlinkAndCancellation(t *testing.T) {
	s := testStore(t)
	root, err := s.openRoot()
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.acquireWriter(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Millisecond)
	defer cancel()
	_, err = s.Put(ctx, bytes.NewReader([]byte("x")))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("did not wait for quota lock: %v", err)
	}
	release()
	root.Close()
	lock := filepath.Join(s.root, ".store-write.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", lock); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("symlinked OCI store lock accepted")
	}
}
func TestStoreRejectsOrphanedImportStagingAtCommit(t *testing.T) {
	s := testStore(t)
	name := filepath.Join(s.root, "tmp", ".ingest-"+strings.Repeat("a", 32))
	if err := os.WriteFile(name, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("import proceeded despite unresolved staging data")
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatal("import removed existing operator-review evidence")
	}
}
