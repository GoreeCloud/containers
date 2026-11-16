package ociimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const (
	maxAuditBlobs            = 4096
	maxAuditTotalBytes int64 = 1 << 30
)

// AuditReport is an aggregate integrity finding; it intentionally avoids
// private blob names, digests, paths, and mutable timestamps.
type AuditReport struct {
	ReadOnly                 bool  `json:"readOnly"`
	Verified                 bool  `json:"verified"`
	BlobCount                int   `json:"blobCount"`
	TotalBytes               int64 `json:"totalBytes"`
	ContentAuthenticated     bool  `json:"contentAuthenticated"`
	SourceProvenanceVerified bool  `json:"sourceProvenanceVerified"`
	PendingImportFiles       int   `json:"pendingImportFiles"`
	PendingImportBytes       int64 `json:"pendingImportBytes"`
	OperatorReviewRequired   bool  `json:"operatorReviewRequired"`
}

// OpenExistingStore never creates or repairs a directory; callers may safely
// use it for observational integrity checks of an already existing store.
func OpenExistingStore(path string) (*Store, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("rootful store inspection prohibited")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("store audit requires a clean absolute path")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return nil, errors.New("symlinked store parent not permitted")
	}
	store := &Store{root: path, maxBytes: maxBlobBytes, maxTotalBytes: maxAuditTotalBytes}
	if err := store.verifyDirectories(); err != nil {
		return nil, err
	}
	return store, nil
}

// Audit validates a bounded set of committed SHA256 blobs. A successful audit
// proves matching bytes for the observed file descriptors at that moment only.
// The store is not frozen against subsequent same-user modification.
func (s *Store) Audit(ctx context.Context) (AuditReport, error) {
	var empty AuditReport
	if s == nil || ctx == nil {
		return empty, errors.New("store and context required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	root, err := s.openRoot()
	if err != nil {
		return empty, err
	}
	defer root.Close()
	before, err := root.Lstat("blobs/sha256")
	if err != nil {
		return empty, err
	}
	dir, err := root.Open("blobs/sha256")
	if err != nil {
		return empty, err
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.IsDir() {
		return empty, errors.New("blob directory changed during audit")
	}
	names, err := dir.Readdirnames(maxAuditBlobs + 1)
	if err != nil && err != io.EOF {
		return empty, err
	}
	if len(names) > maxAuditBlobs {
		return empty, errors.New("image store audit entry limit exceeded")
	}
	sort.Strings(names)
	result := AuditReport{ReadOnly: true, Verified: false,
		ContentAuthenticated: false, SourceProvenanceVerified: false}
	buffer := make([]byte, 32<<10)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return empty, err
		}
		digest := "sha256:" + name
		if !digestPattern.MatchString(digest) {
			return empty, errors.New("image store contains invalid blob filename")
		}
		relative := "blobs/sha256/" + name
		info, err := root.Lstat(relative)
		if err != nil {
			return empty, err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > s.maxBytes {
			return empty, errors.New("image store contains unsafe or oversized blob")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Nlink != 1 {
			return empty, errors.New("image blob must not be hardlinked outside store")
		}
		if info.Size() > maxAuditTotalBytes-result.TotalBytes {
			return empty, errors.New("store audit aggregate quota exceeded")
		}
		file, err := root.Open(relative)
		if err != nil {
			return empty, err
		}
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
			file.Close()
			return empty, errors.New("image blob changed while opening")
		}
		hash := sha256.New()
		var length int64
		idle := 0
		for {
			if err := ctx.Err(); err != nil {
				file.Close()
				return empty, err
			}
			n, readErr := file.Read(buffer)
			if n < 0 || n > len(buffer) {
				file.Close()
				return empty, errors.New("invalid stored blob read count")
			}
			if n > 0 {
				idle = 0
				length += int64(n)
				if length > info.Size() {
					file.Close()
					return empty, errors.New("stored blob grew during audit")
				}
				hash.Write(buffer[:n])
			} else if readErr == nil {
				idle++
				if idle > 100 {
					file.Close()
					return empty, io.ErrNoProgress
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				file.Close()
				return empty, fmt.Errorf("stored blob read: %w", readErr)
			}
		}
		if err := file.Close(); err != nil {
			return empty, err
		}
		if length != info.Size() || hex.EncodeToString(hash.Sum(nil)) != name {
			return empty, errors.New("stored blob failed digest verification")
		}
		result.TotalBytes += length
		result.BlobCount++
	}
	if err := s.inspectPendingImports(ctx, root, &result); err != nil {
		return empty, err
	}
	// A pending import may still be active or may be an interrupted write.
	// It is never safe to declare the entire store fully verified until an
	// operator has resolved the staging state.
	result.Verified = !result.OperatorReviewRequired
	return result, nil
}

// inspectPendingImports reports leftover OR active staging files without
// altering, deleting, opening for writes, or assuming they are orphaned.
// A concurrent import may legitimately appear here. Its state requires
// operator review; there is intentionally no automatic GC/rollback.
func (s *Store) inspectPendingImports(ctx context.Context, root *os.Root, report *AuditReport) error {
	before, err := root.Lstat("tmp")
	if err != nil || privateStoreInfo(before) != nil {
		return errors.New("untrusted image temporary directory")
	}
	dir, err := root.Open("tmp")
	if err != nil {
		return err
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.IsDir() {
		return errors.New("image temporary directory changed during audit")
	}
	names, err := dir.Readdirnames(maxAuditBlobs + 1)
	if err != nil && err != io.EOF {
		return err
	}
	if len(names) > maxAuditBlobs {
		return errors.New("too many pending OCI import entries")
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.HasPrefix(name, ".ingest-") || len(name) != 40 {
			return errors.New("unknown file in image import staging directory")
		}
		if _, err := hex.DecodeString(name[8:]); err != nil {
			return errors.New("invalid OCI import temporary filename")
		}
		relative := "tmp/" + name
		info, err := root.Lstat(relative)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > s.maxBytes || info.Mode().Perm()&0077 != 0 {
			return errors.New("invalid OCI import temporary file")
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
			return errors.New("OCI import temporary file ownership or links invalid")
		}
		if info.Size() > maxAuditTotalBytes-report.PendingImportBytes {
			return errors.New("OCI import temporary data exceeds inspection quota")
		}
		report.PendingImportFiles++
		report.PendingImportBytes += info.Size()
	}
	report.OperatorReviewRequired = report.PendingImportFiles > 0
	return nil
}
