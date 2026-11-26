package volume

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/GoreeCloud/containers/internal/regularinput"
)

const maxBackupEntries = 4096
const maxBackupBytes int64 = 256 << 20

type BackupReport struct {
	Directory             string `json:"directory"`
	ContentSHA256         string `json:"contentSHA256"`
	Files                 int    `json:"files"`
	Bytes                 int64  `json:"bytes"`
	Restored              bool   `json:"restored"`
	ApplicationConsistent bool   `json:"applicationConsistent"`
	MountVerified         bool   `json:"mountVerified"`
}

func mountIdentity(f *os.File) (uint64, error) {
	data, e := os.Open(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
	if e != nil {
		return 0, e
	}
	defer data.Close()
	raw, e := io.ReadAll(io.LimitReader(data, 65537))
	if e != nil || len(raw) > 65536 {
		return 0, errors.New("mount identity unavailable")
	}
	var id uint64
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "mnt_id:") {
			n, e := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "mnt_id:")), 10, 64)
			if e != nil || n == 0 || id != 0 {
				return 0, errors.New("invalid mount identity")
			}
			id = n
		}
	}
	if id == 0 {
		return 0, errors.New("missing mount identity")
	}
	return id, nil
}
func rootMount(r *os.Root) (uint64, error) {
	f, e := r.Open(".")
	if e != nil {
		return 0, e
	}
	defer f.Close()
	return mountIdentity(f)
}
func safePayload(i os.FileInfo) bool {
	if i == nil || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || i.Mode().Perm()&0077 != 0 {
		return false
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid()) && (i.IsDir() || (i.Mode().IsRegular() && st.Nlink == 1))
}
func samePayload(a, b os.FileInfo) bool {
	if !safePayload(a) || !safePayload(b) || !os.SameFile(a, b) || a.Size() != b.Size() || a.Mode() != b.Mode() {
		return false
	}
	return a.Sys().(*syscall.Stat_t).Ctim == b.Sys().(*syscall.Stat_t).Ctim
}

// Copy a quiesced, owner-private regular-file/directory subset. The digest
// covers normalized private modes, relative names, lengths and bytes. Never
// follow links, cross open-file mount boundaries or copy extended metadata.
func copyPayload(ctx context.Context, src, dst *os.Root) (BackupReport, error) {
	var report BackupReport
	sm, e := rootMount(src)
	if e != nil {
		return report, e
	}
	dm, e := rootMount(dst)
	if e != nil {
		return report, e
	}
	h := sha256.New()
	entries := 0
	var walk func(string) error
	walk = func(rel string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		info, e := src.Lstat(rel)
		if e != nil || !safePayload(info) {
			return errors.New("unsafe volume payload entry")
		}
		entries++
		if entries > maxBackupEntries {
			return errors.New("backup entry bound exceeded")
		}
		if info.IsDir() {
			if rel != "." {
				if e = dst.Mkdir(rel, 0700); e != nil {
					return e
				}
			}
			f, e := src.Open(rel)
			if e != nil {
				return e
			}
			defer f.Close()
			opened, e := f.Stat()
			if e != nil || !samePayload(info, opened) {
				return errors.New("volume directory changed")
			}
			m, e := mountIdentity(f)
			if e != nil || m != sm {
				return errors.New("volume source mount boundary crossed")
			}
			names, e := f.Readdirnames(maxBackupEntries + 1)
			if e != nil && e != io.EOF {
				return e
			}
			if len(names) > maxBackupEntries {
				return errors.New("backup directory bound exceeded")
			}
			sort.Strings(names)
			fmt.Fprintf(h, "d:%d:%s\n", len(rel), rel)
			for _, n := range names {
				if e = walk(filepath.Join(rel, n)); e != nil {
					return e
				}
			}
			after, e := src.Lstat(rel)
			if e != nil || !samePayload(info, after) {
				return errors.New("volume directory changed during copy")
			}
			d, e := dst.Open(rel)
			if e != nil {
				return e
			}
			defer d.Close()
			m, e = mountIdentity(d)
			if e != nil || m != dm {
				return errors.New("volume destination mount boundary crossed")
			}
			return d.Sync()
		}
		if info.Size() < 0 || info.Size() > maxBackupBytes-report.Bytes {
			return errors.New("volume backup byte bound exceeded")
		}
		in, e := regularinput.Open(src, rel, info)
		if e != nil {
			return e
		}
		defer in.Close()
		m, e := mountIdentity(in)
		if e != nil || m != sm {
			return errors.New("volume source mount boundary crossed")
		}
		out, e := dst.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if e != nil {
			return e
		}
		defer out.Close()
		m, e = mountIdentity(out)
		if e != nil || m != dm {
			return errors.New("volume destination mount boundary crossed")
		}
		fmt.Fprintf(h, "f:%d:%s:600:%d\n", len(rel), rel, info.Size())
		n, e := io.CopyN(io.MultiWriter(out, h), in, info.Size())
		if e != nil || n != info.Size() {
			return errors.New("volume file changed or truncated")
		}
		if e = out.Sync(); e != nil {
			return e
		}
		after, e := in.Stat()
		if e != nil || !samePayload(info, after) {
			return errors.New("volume file changed during copy")
		}
		path, e := src.Lstat(rel)
		if e != nil || !samePayload(info, path) {
			return errors.New("volume file path changed")
		}
		report.Files++
		report.Bytes += n
		return ctx.Err()
	}
	if e = walk("."); e != nil {
		return BackupReport{}, e
	}
	report.ContentSHA256 = fmt.Sprintf("sha256:%x", h.Sum(nil))
	return report, nil
}
func separate(a, b string) bool {
	for _, p := range [][2]string{{a, b}, {b, a}} {
		rel, e := filepath.Rel(p[0], p[1])
		if e != nil || filepath.IsLocal(rel) {
			return false
		}
	}
	return true
}
func publishCopy(ctx context.Context, src, base *os.Root, basePath, name, expected string, restore bool) (report BackupReport, err error) {
	if e := base.Mkdir(name, 0700); e != nil {
		return report, e
	}
	before, e := base.Lstat(name)
	if e != nil {
		return report, e
	}
	keep := false
	defer func() {
		if !keep {
			current, e := base.Lstat(name)
			if e != nil || !os.SameFile(before, current) {
				err = errors.Join(err, errors.New("new volume tree identity changed; review required"))
				return
			}
			err = errors.Join(err, base.RemoveAll(name))
		}
	}()
	dst, e := base.OpenRoot(name)
	if e != nil {
		return report, e
	}
	defer dst.Close()
	actual, e := dst.Stat(".")
	if e != nil || !os.SameFile(before, actual) {
		return report, errors.New("new volume tree changed")
	}
	bm, e := rootMount(base)
	if e != nil {
		return report, e
	}
	dm, e := rootMount(dst)
	if e != nil || bm != dm {
		return report, errors.New("new volume root mount changed")
	}
	report, e = copyPayload(ctx, src, dst)
	if e != nil {
		return BackupReport{}, e
	}
	if expected != "" && report.ContentSHA256 != expected {
		return BackupReport{}, errors.New("backup content pin mismatch")
	}
	baseInfo, e := base.Stat(".")
	if e != nil {
		return BackupReport{}, e
	}
	current, e := os.Lstat(basePath)
	if e != nil || !os.SameFile(baseInfo, current) {
		return BackupReport{}, errors.New("volume parent changed")
	}
	current, e = base.Lstat(name)
	if e != nil || !os.SameFile(before, current) {
		return BackupReport{}, errors.New("volume tree changed")
	}
	f, e := base.Open(".")
	if e != nil {
		return BackupReport{}, e
	}
	if e = errors.Join(f.Sync(), f.Close()); e != nil {
		return BackupReport{}, e
	}
	if e = ctx.Err(); e != nil {
		return BackupReport{}, e
	}
	keep = true
	report.Directory = filepath.Join(basePath, name)
	report.Restored = restore
	return report, nil
}
func Backup(ctx context.Context, store, id, parent string) (BackupReport, error) {
	if ctx == nil || !validID.MatchString(id) || !separate(store, parent) {
		return BackupReport{}, errors.New("separate private backup parent and volume required")
	}
	r, e := open(store)
	if e != nil {
		return BackupReport{}, e
	}
	defer r.Close()
	lock, e := lockStore(ctx, r, syscall.LOCK_SH)
	if e != nil {
		return BackupReport{}, e
	}
	defer lock.Close()
	before, e := r.Lstat(id)
	if e != nil || !privateDir(before) {
		return BackupReport{}, errors.New("invalid source volume")
	}
	source, e := r.OpenRoot(id)
	if e != nil {
		return BackupReport{}, e
	}
	defer source.Close()
	actual, e := source.Stat(".")
	if e != nil || !os.SameFile(before, actual) {
		return BackupReport{}, errors.New("source volume changed")
	}
	rm, e := rootMount(r)
	if e != nil {
		return BackupReport{}, e
	}
	sm, e := rootMount(source)
	if e != nil || rm != sm {
		return BackupReport{}, errors.New("source volume mount boundary crossed")
	}
	dest, e := open(parent)
	if e != nil {
		return BackupReport{}, e
	}
	defer dest.Close()
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return BackupReport{}, e
	}
	return publishCopy(ctx, source, dest, parent, fmt.Sprintf("backup-%x", nonce[:]), "", false)
}
func Restore(ctx context.Context, store, id, backup, digest string) (BackupReport, error) {
	if ctx == nil || !validID.MatchString(id) || len(digest) != 71 || !strings.HasPrefix(digest, "sha256:") || !separate(store, backup) {
		return BackupReport{}, errors.New("separate pinned backup and fresh volume ID required")
	}
	for _, c := range digest[7:] {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return BackupReport{}, errors.New("invalid backup digest")
		}
	}
	source, e := open(backup)
	if e != nil {
		return BackupReport{}, e
	}
	defer source.Close()
	dest, e := open(store)
	if e != nil {
		return BackupReport{}, e
	}
	defer dest.Close()
	lock, e := lockStore(ctx, dest, syscall.LOCK_EX)
	if e != nil {
		return BackupReport{}, e
	}
	defer lock.Close()
	count, e := countLocked(ctx, dest)
	if e != nil {
		return BackupReport{}, e
	}
	if count >= MaxVolumes {
		return BackupReport{}, errors.New("volume count capacity reached")
	}
	return publishCopy(ctx, source, dest, store, id, digest, true)
}
