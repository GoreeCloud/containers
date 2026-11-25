// Package regularinput admits only the regular file previously inspected.
// Nonblocking open avoids hanging on a substituted FIFO; no final symlink
// is admitted. This is not an immutable snapshot or a device sandbox.
package regularinput

import (
	"errors"
	"os"
	"syscall"
)

func Open(root *os.Root, name string, expected os.FileInfo) (*os.File, error) {
	if root == nil || expected == nil || !expected.Mode().IsRegular() {
		return nil, errors.New("regular input metadata required")
	}
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || !os.SameFile(expected, before) || before.Size() != expected.Size() {
		return nil, errors.New("regular input path changed before opening")
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) || actual.Size() != expected.Size() {
		f.Close()
		return nil, errors.New("regular input changed during opening")
	}
	after, err := root.Lstat(name)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(actual, after) || after.Size() != expected.Size() {
		f.Close()
		return nil, errors.New("regular input path changed after opening")
	}
	return f, nil
}
