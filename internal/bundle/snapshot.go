package bundle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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

const MaxSnapshotFiles = 4096
const MaxSnapshotBytes int64 = 256 << 20

type SnapshotReport struct {
	DelegatedGuest      bool   `json:"delegatedGuest,omitempty"`
	Directory           string `json:"directory"`
	ContentSHA256       string `json:"contentSHA256"`
	Files               int    `json:"files"`
	Bytes               int64  `json:"bytes"`
	Immutable           bool   `json:"immutable"`
	ExecutionAuthorized bool   `json:"executionAuthorized"`
}

// Snapshot copies a restricted, caller-owned local bundle into an exclusive
// private directory and validates the resulting bundle. Source links, hard
// links, special files, filesystem crossings and privileged bits are refused.
// Neither this digest nor a private copy authenticates source or defeats a
// hostile same-UID actor. Never automatically launch the resulting directory.
func Snapshot(ctx context.Context, source, parent string) (report SnapshotReport, err error) {
	return snapshot(ctx, source, parent, false)
}

// SnapshotDelegated normalizes only a new private copy for a nonroot guest.
// Outer directory and config stay owner-private; guest-readable payload is
// reachable only through that private directory until runtime admission.
func SnapshotDelegated(ctx context.Context, source, parent string) (SnapshotReport, error) {
	return snapshot(ctx, source, parent, true)
}
func snapshot(ctx context.Context, source, parent string, delegated bool) (report SnapshotReport, err error) {

	if ctx == nil || os.Geteuid() == 0 {
		return report, errors.New("nonroot snapshot context required")
	}
	if err = ctx.Err(); err != nil {
		return report, err
	}
	for _, p := range []string{source, parent} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return report, errors.New("clean absolute snapshot paths required")
		}
		real, e := filepath.EvalSymlinks(p)
		if e != nil || real != p {
			return report, errors.New("symlinked snapshot path")
		}
	}
	if rel, e := filepath.Rel(source, parent); e == nil && filepath.IsLocal(rel) {
		return report, errors.New("destination must not be inside source")
	}
	sourceBefore, e := os.Lstat(source)
	if e != nil {
		return report, e
	}
	if delegated && sourceBefore.Mode().Perm() != 0700 {
		return report, errors.New("delegated source bundle must be private")
	}
	src, e := os.OpenRoot(source)
	if e != nil {
		return report, e
	}
	defer src.Close()
	srcInfo, e := src.Stat(".")
	if e != nil {
		return report, e
	}
	if !os.SameFile(sourceBefore, srcInfo) {
		return report, errors.New("source root changed while opening")
	}
	sourceMount, e := rootMountID(src)
	if e != nil {
		return report, e
	}
	dev, e := snapshotInfo(srcInfo, true, 0)
	if e != nil {
		return report, e
	}
	parentInfo, e := os.Lstat(parent)
	if e != nil {
		return report, e
	}
	if _, e = snapshotInfo(parentInfo, true, 0); e != nil || parentInfo.Mode().Perm()&0077 != 0 {
		return report, errors.New("private owner-only snapshot parent required")
	}
	base, e := os.OpenRoot(parent)
	if e != nil {
		return report, e
	}
	defer base.Close()
	openedParent, e := base.Stat(".")
	if e != nil || !os.SameFile(parentInfo, openedParent) {
		return report, errors.New("snapshot parent changed while opening")
	}
	parentMount, e := rootMountID(base)
	if e != nil {
		return report, e
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return report, e
	}
	name := "bundle-" + hex.EncodeToString(nonce[:])
	if e = base.Mkdir(name, 0700); e != nil {
		return report, e
	}
	dir := filepath.Join(parent, name)
	initial, e := base.Lstat(name)
	if e != nil {
		return report, e
	}
	keep := false
	defer func() {
		if !keep {
			current, e := base.Lstat(name)
			if e != nil || !os.SameFile(initial, current) {
				err = errors.Join(err, errors.New("snapshot identity changed; review required"))
				return
			}
			if e = base.RemoveAll(name); e != nil {
				err = errors.Join(err, errors.New("snapshot cleanup failed"))
			}
		}
	}()
	dst, e := base.OpenRoot(name)
	if e != nil {
		return report, e
	}
	defer dst.Close()
	opened, e := dst.Stat(".")
	if e != nil || !os.SameFile(initial, opened) {
		return report, errors.New("snapshot destination changed")
	}
	destMount, e := rootMountID(dst)
	if e != nil || destMount != parentMount {
		return report, errors.New("snapshot destination mount changed")
	}
	// Copy the restricted source tree without following links. Validate the
	// copied config and rootfs before exposing the result.
	digest := sha256.New()
	if delegated {
		digest.Write([]byte("goreecloud-delegated-snapshot-v1\x00"))
	}
	entries := 0
	var walk func(string) error
	walk = func(rel string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		info, e := src.Lstat(rel)
		if e != nil {
			return e
		}
		if _, e = snapshotInfo(info, info.IsDir(), dev); e != nil {
			return e
		}
		entries++
		if entries > MaxSnapshotFiles {
			return errors.New("snapshot entry quota exceeded")
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
			opened, e := f.Stat()
			if e != nil || !os.SameFile(info, opened) {
				f.Close()
				return errors.New("snapshot source directory changed")
			}
			if m, e := fileMountID(f); e != nil || m != sourceMount {
				f.Close()
				return errors.New("snapshot source mount boundary crossed")
			}
			names, e := f.ReadDir(MaxSnapshotFiles + 1)
			f.Close()
			if e != nil && e != io.EOF {
				return e
			}
			if len(names) > MaxSnapshotFiles {
				return errors.New("snapshot directory quota exceeded")
			}
			// directory enumeration order is not deterministic on all filesystems.
			sort.Slice(names, func(i, j int) bool { return names[i].Name() < names[j].Name() })
			fmt.Fprintf(digest, "d:%d:%s\n", len(rel), rel)
			for _, entry := range names {
				if e = walk(filepath.Join(rel, entry.Name())); e != nil {
					return e
				}
			}
			after, e := src.Lstat(rel)
			if e != nil || !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) {
				return errors.New("snapshot directory changed")
			}
			df, e := dst.Open(rel)
			if e != nil {
				return e
			}
			if m, e := fileMountID(df); e != nil || m != destMount {
				df.Close()
				return errors.New("snapshot destination mount boundary crossed")
			}
			if delegated && (rel == "rootfs" || strings.HasPrefix(rel, "rootfs/")) {
				if e = df.Chmod(0755); e != nil {
					df.Close()
					return e
				}
			}
			return errors.Join(df.Sync(), df.Close())
		}
		if info.Size() < 0 || info.Size() > MaxSnapshotBytes-report.Bytes {
			return errors.New("snapshot byte quota exceeded")
		}
		in, e := regularinput.Open(src, rel, info)
		if e != nil {
			return e
		}
		defer in.Close()
		if m, e := fileMountID(in); e != nil || m != sourceMount {
			return errors.New("snapshot source mount boundary crossed")
		}
		mode := os.FileMode(0400)
		if info.Mode().Perm()&0100 != 0 {
			mode = 0500
		}
		if delegated && strings.HasPrefix(rel, "rootfs/") {
			mode |= mode>>3 | mode>>6
		}
		out, e := dst.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if e != nil {
			return e
		}
		if m, e := fileMountID(out); e != nil || m != destMount {
			out.Close()
			return errors.New("snapshot destination mount boundary crossed")
		}
		fmt.Fprintf(digest, "f:%d:%s:%o:%d\n", len(rel), rel, mode, info.Size())
		n, e := io.CopyN(io.MultiWriter(out, digest), in, info.Size())
		if e == nil && n != info.Size() {
			e = io.ErrUnexpectedEOF
		}
		if e == nil && delegated {
			e = out.Chmod(mode)
		}
		if e == nil {
			e = out.Sync()
		}
		if e = errors.Join(e, out.Close()); e != nil {
			return e
		}
		after, e := in.Stat()
		if e != nil || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return errors.New("snapshot file changed")
		}
		pathInfo, e := src.Lstat(rel)
		if e != nil || !os.SameFile(info, pathInfo) {
			return errors.New("snapshot source identity changed")
		}
		report.Files++
		report.Bytes += n
		return ctx.Err()
	}
	if e = walk("."); e != nil {
		return SnapshotReport{}, e
	}
	sourceAfter, e := os.Lstat(source)
	if e != nil || !os.SameFile(sourceBefore, sourceAfter) {
		return SnapshotReport{}, errors.New("source root path changed")
	}
	spec, e := Load(dir)
	if e != nil {
		return SnapshotReport{}, e
	}
	if delegated {
		if e = RequireDelegatedMapping(spec); e != nil {
			return SnapshotReport{}, e
		}
		if e = ValidateDelegatedLayout(ctx, dir, spec); e != nil {
			return SnapshotReport{}, e
		}
	} else if e = RequireSingleCallerMapping(spec, uint32(os.Geteuid()), uint32(os.Getegid())); e != nil {
		return SnapshotReport{}, e
	}
	if e = ValidateRuntimeLayout(dir, spec); e != nil {
		return SnapshotReport{}, e
	}
	if !HasExplicitGuestExecveAllowance(spec) {
		return SnapshotReport{}, errors.New("snapshot requires guest execve allowance")
	}
	after, e := os.Lstat(parent)
	if e != nil || !os.SameFile(parentInfo, after) {
		return SnapshotReport{}, errors.New("snapshot parent changed")
	}
	current, e := os.Lstat(dir)
	if e != nil || !os.SameFile(initial, current) {
		return SnapshotReport{}, errors.New("snapshot directory changed")
	}
	if e = ctx.Err(); e != nil {
		return SnapshotReport{}, e
	}
	df, e := base.Open(".")
	if e != nil {
		return SnapshotReport{}, e
	}
	if e = errors.Join(df.Sync(), df.Close()); e != nil {
		return SnapshotReport{}, e
	}
	report.DelegatedGuest = delegated
	report.Directory = dir
	report.ContentSHA256 = "sha256:" + hex.EncodeToString(digest.Sum(nil))
	keep = true
	return report, nil
}
func snapshotInfo(info os.FileInfo, dir bool, dev uint64) (uint64, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0022 != 0 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || (!dir && (!info.Mode().IsRegular() || st.Nlink != 1)) || (dir && !info.IsDir()) || (dev != 0 && uint64(st.Dev) != dev) {
		return 0, errors.New("unsafe snapshot entry ownership, mode, link or filesystem")
	}
	return uint64(st.Dev), nil
}

// Linux fdinfo identifies the mount of an already-open file, including
// same-device bind mounts, without trusting a path or parsing mount names.
func rootMountID(r *os.Root) (uint64, error) {
	f, e := r.Open(".")
	if e != nil {
		return 0, e
	}
	defer f.Close()
	return fileMountID(f)
}
func fileMountID(f *os.File) (uint64, error) {
	info, e := os.Open(fmt.Sprintf("/proc/self/fdinfo/%d", f.Fd()))
	if e != nil {
		return 0, e
	}
	defer info.Close()
	raw, e := io.ReadAll(io.LimitReader(info, 65537))
	if e != nil || len(raw) > 65536 {
		return 0, errors.New("mount identity unavailable")
	}
	return parseMountID(string(raw))
}
func parseMountID(raw string) (uint64, error) {
	var id uint64
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "mnt_id:") {
			continue
		}
		n, e := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "mnt_id:")), 10, 64)
		if e != nil || n == 0 || id != 0 {
			return 0, errors.New("invalid mount identity")
		}
		id = n
	}
	if id == 0 {
		return 0, errors.New("mount identity missing")
	}
	return id, nil
}
