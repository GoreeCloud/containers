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
	if !filepath.IsAbs(stateRoot) || filepath.Clean(stateRoot) != stateRoot {
		return errors.New("absolute state directory required")
	}
	before, err := capturePrivateRuntimeRoot(stateRoot)
	if err != nil {
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
	if privateStateInfo(checked) != nil || !os.SameFile(before, checked) {
		return errors.New("runtime root changed while opening lock")
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
	// An inode checked before flock can be unlinked or replaced while
	// this process waits for another cooperating writer to release it.
	if err := verifyAcquiredContainerLock(root, lockPath, opened); err != nil {
		return err
	}
	if err := verifyRuntimeDirectories(root, stateRoot, before, "locks", info); err != nil {
		return err
	}
	actionErr := action()
	if err := verifyRuntimeDirectories(root, stateRoot, before, "locks", info); err != nil {
		return errors.Join(actionErr, err)
	}
	if err := verifyAcquiredContainerLock(root, lockPath, opened); err != nil {
		return errors.Join(actionErr, err)
	}
	return actionErr
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

// verifyAcquiredContainerLock detects lock-path replacement after flock.
// This is advisory cooperation, not defense against a malicious same-user
// actor changing a file later or bypassing our lock.
func verifyAcquiredContainerLock(root *os.Root, name string, opened os.FileInfo) error {
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(current, opened) || !current.Mode().IsRegular() ||
		current.Mode().Perm()&0077 != 0 {
		return errors.New("OCI cooperative lock changed during acquisition")
	}
	st, ok := current.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errors.New("OCI cooperative lock owner or hardlink changed during acquisition")
	}
	return nil
}

// Check both externally named and anchored directories before admission and
// after observation; never treat a replacement private root as the old root.
func capturePrivateRuntimeRoot(name string) (os.FileInfo, error) {
	real, err := filepath.EvalSymlinks(name)
	if err != nil || real != name {
		return nil, errors.New("runtime root requires non-symlink absolute path")
	}
	before, err := os.Lstat(name)
	if err != nil || privateStateInfo(before) != nil {
		return nil, errors.New("unsafe runtime root")
	}
	return before, nil
}
func verifyRuntimeDirectories(root *os.Root, name string, before os.FileInfo, sub string, subBefore os.FileInfo) error {
	actual, err := capturePrivateRuntimeRoot(name)
	if err != nil || !os.SameFile(before, actual) {
		return errors.New("runtime root identity changed; recovery review required")
	}
	anchor, err := root.Lstat(".")
	if err != nil || privateStateInfo(anchor) != nil || !os.SameFile(before, anchor) {
		return errors.New("anchored runtime root changed")
	}
	child, err := root.Lstat(sub)
	if err != nil || privateStateInfo(child) != nil || !os.SameFile(subBefore, child) {
		return errors.New("runtime control directory changed; recovery review required")
	}
	external, err := os.Lstat(filepath.Join(name, sub))
	if err != nil || privateStateInfo(external) != nil || !os.SameFile(subBefore, external) {
		return errors.New("external runtime control directory changed")
	}
	return nil
}
