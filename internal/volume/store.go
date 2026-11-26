// Package volume manages caller-owned persistent storage directories, not mounts.
package volume

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
)

var validID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

type Report struct {
	ID      string `json:"id"`
	Exists  bool   `json:"exists"`
	Private bool   `json:"private"`
	Mounted bool   `json:"mountVerified"`
}

func privateDir(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode().Perm() != 0700 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid())
}
func open(parent string) (*os.Root, error) {
	if os.Geteuid() == 0 || !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return nil, errors.New("nonroot clean absolute private volume store required")
	}
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return nil, errors.New("symlinked volume store")
	}
	before, e := os.Lstat(parent)
	if e != nil || !privateDir(before) {
		return nil, errors.New("volume store must be caller-owned mode 0700")
	}
	r, e := os.OpenRoot(parent)
	if e != nil {
		return nil, e
	}
	after, e := r.Stat(".")
	if e != nil || !os.SameFile(before, after) {
		r.Close()
		return nil, errors.New("volume store changed")
	}
	return r, nil
}

// Create refuses existing names. Never removes or resets existing data.
func Create(ctx context.Context, parent, id string) (Report, error) {
	if ctx == nil || !validID.MatchString(id) {
		return Report{}, errors.New("invalid volume ID or context")
	}
	if e := ctx.Err(); e != nil {
		return Report{}, e
	}
	r, e := open(parent)
	if e != nil {
		return Report{}, e
	}
	defer r.Close()
	if e = r.Mkdir(id, 0700); e != nil {
		return Report{}, e
	}
	info, e := r.Lstat(id)
	if e != nil || !privateDir(info) {
		return Report{}, errors.New("created volume requires inspection")
	}
	dir, e := r.Open(".")
	if e != nil {
		return Report{}, e
	}
	e = errors.Join(dir.Sync(), dir.Close())
	if e != nil {
		return Report{}, errors.New("volume creation durability uncertain; inspect existing data")
	}
	return Report{ID: id, Exists: true, Private: true}, nil
}
func Inspect(ctx context.Context, parent, id string) (Report, error) {
	if ctx == nil || !validID.MatchString(id) {
		return Report{}, errors.New("invalid volume ID or context")
	}
	if e := ctx.Err(); e != nil {
		return Report{}, e
	}
	r, e := open(parent)
	if e != nil {
		return Report{}, e
	}
	defer r.Close()
	info, e := r.Lstat(id)
	if os.IsNotExist(e) {
		return Report{ID: id}, nil
	}
	if e != nil || !privateDir(info) {
		return Report{}, errors.New("unsafe volume directory")
	}
	return Report{ID: id, Exists: true, Private: true}, nil
}

// Count is deliberately aggregate-only and does not read private payloads.
func Count(ctx context.Context, parent string) (int, error) {
	if ctx == nil {
		return 0, errors.New("context required")
	}
	r, e := open(parent)
	if e != nil {
		return 0, e
	}
	defer r.Close()
	f, e := r.Open(".")
	if e != nil {
		return 0, e
	}
	defer f.Close()
	names, e := f.Readdirnames(1025)
	if e != nil && e != io.EOF {
		return 0, e
	}
	if len(names) > 1024 {
		return 0, errors.New("volume inventory exceeds bound")
	}
	for _, name := range names {
		if e = ctx.Err(); e != nil {
			return 0, e
		}
		info, e := r.Lstat(name)
		if e != nil || !validID.MatchString(name) || !privateDir(info) {
			return 0, errors.New("unrecognized or unsafe volume entry")
		}
	}
	return len(names), nil
}
