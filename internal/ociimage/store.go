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
	"syscall"
)

// Store manages bounded, user-private, digest-addressed bytes only.
// It never unpacks archives, downloads images, or starts containers.
type Store struct {
	root     string
	maxBytes int64
}
type BlobInfo struct {
	Digest string
	Size   int64
}

// OpenStore creates only the requested private user-owned store.
// Caller supplies an absolute, non-symlink path with an existing parent.
func OpenStore(path string) (*Store, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("rootful image store is prohibited")
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("image store requires a clean absolute path")
	}
	parent := filepath.Dir(path)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return nil, errors.New("symlinked store parent is prohibited")
	}
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	if err := privateStoreDir(path); err != nil {
		return nil, err
	}
	for _, sub := range []string{"tmp", "blobs", "blobs/sha256"} {
		dir := filepath.Join(path, sub)
		if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err := privateStoreDir(dir); err != nil {
			return nil, err
		}
	}
	return &Store{root: path, maxBytes: maxBlobBytes}, nil
}
func privateStoreDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("image store requires a private non-symlink directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("image store requires invoking user ownership")
	}
	return nil
}
func (s *Store) verifyDirectories() error {
	for _, sub := range []string{"", "tmp", "blobs", "blobs/sha256"} {
		if err := privateStoreDir(filepath.Join(s.root, sub)); err != nil {
			return err
		}
	}
	return nil
}

// Put atomically adds verified bytes without overwriting existing content.
// It streams into a user-private temporary file and hard-links it only after
// synchronization and digest completion.
func (s *Store) Put(ctx context.Context, source io.Reader) (BlobInfo, error) {
	var empty BlobInfo
	if ctx == nil || source == nil {
		return empty, errors.New("context and input required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := s.verifyDirectories(); err != nil {
		return empty, err
	}
	temporary, err := os.CreateTemp(filepath.Join(s.root, "tmp"), ".ingest-")
	if err != nil {
		return empty, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	h := sha256.New()
	buffer := make([]byte, 32<<10)
	var length int64
	idle := 0
	for {
		if err := ctx.Err(); err != nil {
			temporary.Close()
			return empty, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			idle = 0
			length += int64(count)
			if length > s.maxBytes {
				temporary.Close()
				return empty, errors.New("image blob exceeds preview quota")
			}
			written, writeErr := temporary.Write(buffer[:count])
			if writeErr != nil || written != count {
				temporary.Close()
				return empty, errors.New("image blob write failed")
			}
			h.Write(buffer[:count])
		} else if readErr == nil {
			idle++
			if idle > 100 {
				temporary.Close()
				return empty, io.ErrNoProgress
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			temporary.Close()
			return empty, readErr
		}
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return empty, err
	}
	if err := temporary.Close(); err != nil {
		return empty, err
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	destination := filepath.Join(s.root, "blobs", "sha256", digest[len("sha256:"):])
	info := BlobInfo{Digest: digest, Size: length}
	if err := s.verifyDirectories(); err != nil {
		return empty, err
	}
	if err := os.Link(name, destination); err != nil {
		if !os.IsExist(err) {
			return empty, fmt.Errorf("unable to store image blob: %w", err)
		}
		size, checkErr := s.Verify(ctx, digest)
		if checkErr != nil || size != length {
			return empty, errors.New("preexisting image blob is corrupt or inconsistent")
		}
		return info, nil
	}
	directory, err := os.Open(filepath.Join(s.root, "blobs", "sha256"))
	if err != nil {
		return empty, err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return empty, syncErr
	}
	if closeErr != nil {
		return empty, closeErr
	}
	return info, nil
}

// Verify checks bytes against a caller-supplied immutable sha256 descriptor.
// The caller must not treat the path or a separate post-verification open as
// race-free. Descriptor-based storage hardening is required before deployment.
func (s *Store) Verify(ctx context.Context, digest string) (int64, error) {
	if ctx == nil {
		return 0, errors.New("context required")
	}
	if !digestPattern.MatchString(digest) {
		return 0, errors.New("invalid image blob digest")
	}
	if err := s.verifyDirectories(); err != nil {
		return 0, err
	}
	path := filepath.Join(s.root, "blobs", "sha256", digest[len("sha256:"):])
	fi, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > s.maxBytes {
		return 0, errors.New("nonregular or oversize stored blob")
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hash := sha256.New()
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n, readErr := f.Read(buffer)
		if n > 0 {
			total += int64(n)
			if total > s.maxBytes {
				return 0, errors.New("stored blob grew past quota")
			}
			hash.Write(buffer[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, readErr
		}
	}
	if total != fi.Size() || hex.EncodeToString(hash.Sum(nil)) != digest[len("sha256:"):] {
		return 0, errors.New("stored blob digest or size mismatch")
	}
	return total, nil
}
