package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// WithContainerLock serializes cooperating GoreeCloud OCI mutations for one
// ID across distinct CLI processes. It is NOT an OCI runtime global lock,
// cannot prevent unrelated crun/runc commands, and does not replace recovery.
func WithContainerLock(ctx context.Context, stateRoot, id string, action func() error) error {
	if ctx == nil || action == nil {
		return errors.New("context and operation required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ValidateID(id); err != nil {
		return err
	}
	if !filepath.IsAbs(stateRoot) {
		return errors.New("absolute state directory required")
	}
	if err := privateDir(stateRoot); err != nil {
		return err
	}
	root, err := os.OpenRoot(stateRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	checked, err := root.Lstat(".")
	if err != nil {
		return err
	}
	if err := privateStateInfo(checked); err != nil {
		return err
	}
	info, err := root.Lstat("locks")
	if err != nil {
		return fmt.Errorf("runtime locks directory: %w", err)
	}
	if err := privateStateInfo(info); err != nil {
		return err
	}
	// An O_NOFOLLOW lock path and private parent prevent accidental
	// cooperative locking on a symlink to an unrelated file.
	lockPath := "locks/" + id + ".lock"
	file, err := root.OpenFile(lockPath, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return fmt.Errorf("open private OCI lock: %w", err)
	}
	defer file.Close()
	link, err := root.Lstat(lockPath)
	if err != nil {
		return err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(link, opened) || !opened.Mode().IsRegular() ||
		opened.Mode().Perm()&0077 != 0 {
		return errors.New("unsafe or replaced OCI lock file")
	}
	stat, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || stat.Nlink != 1 {
		return errors.New("OCI lock file must be private, user-owned and unlinked from other paths")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN && err != syscall.EINTR {
			return fmt.Errorf("OCI lock operation failed: %w", err)
		}
		timer := time.NewTimer(30 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	if err := ctx.Err(); err != nil {
		return err
	}
	return action()
}

// privateStateInfo enforces only local ownership/permissions; it cannot
// attest to host filesystem/mount integrity.
func privateStateInfo(info os.FileInfo) error {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("OCI lock parent is not a private directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("OCI lock parent not caller-owned")
	}
	return nil
}
