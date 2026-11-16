package ociimage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingStoreReadOnlyAudit(t *testing.T) {
	s := testStore(t)
	if _, err := s.Put(context.Background(), bytes.NewReader([]byte("first synthetic blob"))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(), bytes.NewReader([]byte("second synthetic blob"))); err != nil {
		t.Fatal(err)
	}
	existing, err := OpenExistingStore(s.root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := existing.Audit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReadOnly || !report.Verified || report.BlobCount != 2 ||
		report.ContentAuthenticated || report.SourceProvenanceVerified {
		t.Fatalf("misleading audit response: %+v", report)
	}
	if report.TotalBytes != int64(len("first synthetic blob")+len("second synthetic blob")) {
		t.Fatalf("unexpected byte total: %+v", report)
	}
}
func TestAuditDoesNotCreateMissingStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-a-store")
	if _, err := OpenExistingStore(missing); err == nil {
		t.Fatal("accepted missing image store")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("audit unexpectedly created an image store")
	}
}
func TestAuditRejectsTamperedBlobAndInvalidFilename(t *testing.T) {
	s := testStore(t)
	blob, err := s.Put(context.Background(), bytes.NewReader([]byte("good synthetic")))
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(s.root, "blobs", "sha256", strings.TrimPrefix(blob.Digest, "sha256:"))
	if err := os.WriteFile(location, []byte("BAD"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Audit(context.Background()); err == nil {
		t.Fatal("corrupted blob accepted")
	}
	if err := os.Remove(location); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "blobs", "sha256", "README"), []byte("unsafe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Audit(context.Background()); err == nil {
		t.Fatal("invalid blob name accepted")
	}
}
func TestAuditRejectsHardlinkAndSymlink(t *testing.T) {
	s := testStore(t)
	blob, err := s.Put(context.Background(), bytes.NewReader([]byte("opaque bytes")))
	if err != nil {
		t.Fatal(err)
	}
	location := filepath.Join(s.root, "blobs", "sha256", strings.TrimPrefix(blob.Digest, "sha256:"))
	alias := filepath.Join(s.root, "alias")
	if err := os.Link(location, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Audit(context.Background()); err == nil {
		t.Fatal("hardlink accepted")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(location); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", location); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Audit(context.Background()); err == nil {
		t.Fatal("symlinked blob accepted")
	}
}
func TestAuditCancellationAndEmptyStore(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Audit(ctx); err == nil {
		t.Fatal("canceled audit allowed")
	}
	report, err := s.Audit(context.Background())
	if err != nil || !report.Verified || report.BlobCount != 0 {
		t.Fatalf("empty store audit failed: %+v %v", report, err)
	}
}
