package ociimage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// acquireWriter serializes cooperating imports across processes on a single
// private, inode-checked advisory flock. Uncooperative direct filesystem
// writes and hostile same-user replacements remain outside this guarantee.
func (s *Store) acquireWriter(ctx context.Context, root *os.Root) (func(), error) {
	if ctx == nil {
		return nil, errors.New("context required")
	}
	name := ".store-write.lock"
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open OCI store quota lock: %w", err)
	}
	fail := func(err error) (func(), error) { file.Close(); return nil, err }
	before, err := root.Lstat(name)
	if err != nil {
		return fail(err)
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() ||
		opened.Mode().Perm()&0077 != 0 {
		return fail(errors.New("unsafe OCI store quota lock"))
	}
	st, ok := opened.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return fail(errors.New("OCI store quota lock must be user-owned and privately linked"))
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN && err != syscall.EINTR {
			return fail(fmt.Errorf("OCI store quota lock failed: %w", err))
		}
		timer := time.NewTimer(30 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail(ctx.Err())
		case <-timer.C:
		}
	}
	// A cooperating writer may have waited for another process. Recheck
	// the lock pathname AFTER acquiring flock: otherwise unlink/replace
	// during that wait could admit a writer on a stale inode.
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(current, opened) {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		return fail(errors.New("OCI store lock was replaced while waiting"))
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}
