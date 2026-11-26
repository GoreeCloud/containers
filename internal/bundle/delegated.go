package bundle

import (
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const delegatedGuestID uint32 = 1000

type DelegatedPreflight struct {
	ReadOnly             bool `json:"readOnly"`
	MappingVerified      bool `json:"mappingVerified"`
	LayoutVerified       bool `json:"layoutVerified"`
	HelpersVerified      bool `json:"helpersVerified"`
	GuestNonRoot         bool `json:"guestNonRoot"`
	RuntimeInvoked       bool `json:"runtimeInvoked"`
	IsolationVerified    bool `json:"isolationVerified"`
	DeploymentAuthorized bool `json:"deploymentAuthorized"`
}
type allocation struct {
	owner      string
	start, end uint64
}

func parseAllocations(raw string) ([]allocation, error) {
	if len(raw) > 1<<20 {
		return nil, errors.New("subordinate allocation file too large")
	}
	out := []allocation{}
	for _, line := range strings.Split(raw, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 || parts[0] == "" || strings.ContainsAny(parts[0], " \t\r") {
			return nil, errors.New("invalid subordinate allocation record")
		}
		a, e := strconv.ParseUint(parts[1], 10, 32)
		if e != nil {
			return nil, errors.New("invalid subordinate start")
		}
		n, e := strconv.ParseUint(parts[2], 10, 32)
		if e != nil || a < 65536 || n == 0 || a+n > 1<<32 {
			return nil, errors.New("unsafe subordinate allocation")
		}
		out = append(out, allocation{parts[0], a, a + n})
		if len(out) > 8192 {
			return nil, errors.New("allocation quota exceeded")
		}
	}
	return out, nil
}
func allocated(raw, name string, uid, requested uint32) error {
	ranges, e := parseAllocations(raw)
	if e != nil {
		return e
	}
	if requested < 65536 || requested == ^uint32(0) || requested == uid {
		return errors.New("invalid subordinate identity")
	}
	matches := 0
	for _, r := range ranges {
		if uint64(requested) < r.start || uint64(requested) >= r.end {
			continue
		}
		if r.owner != name && r.owner != strconv.FormatUint(uint64(uid), 10) {
			return errors.New("subordinate identity overlaps another account")
		}
		matches++
	}
	if matches != 1 {
		return errors.New("exactly one caller subordinate allocation required")
	}
	return nil
}
func exactDelegatedMaps(m []IDMap, caller uint32) (uint32, error) {
	if len(m) != 2 || caller == 0 || !validMaps(m) {
		return 0, errors.New("two single-ID mappings required")
	}
	var root, guest bool
	var host uint32
	for _, x := range m {
		if x.Size != 1 {
			return 0, errors.New("single-ID mappings only")
		}
		switch x.ContainerID {
		case 0:
			root = x.HostID == caller
		case delegatedGuestID:
			guest = x.HostID >= 65536 && x.HostID != caller
			host = x.HostID
		default:
			return 0, errors.New("unexpected guest identity mapping")
		}
	}
	if !root || !guest {
		return 0, errors.New("caller root and one delegated guest required")
	}
	return host, nil
}
func trustedHostFile(name string, mode os.FileMode, max int64) (string, error) {
	real, e := filepath.EvalSymlinks(name)
	if e != nil || real != name {
		return "", errors.New("untrusted host policy path")
	}
	for p := filepath.Dir(name); ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e != nil {
			return "", e
		}
		st, ok := i.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || !i.IsDir() || i.Mode().Perm()&0022 != 0 {
			return "", errors.New("unsafe host policy ancestor")
		}
		if p == "/" {
			break
		}
	}
	r, e := os.OpenRoot(filepath.Dir(name))
	if e != nil {
		return "", e
	}
	defer r.Close()
	b := filepath.Base(name)
	i, e := r.Lstat(b)
	if e != nil {
		return "", e
	}
	st, ok := i.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != 0 || st.Gid != 0 || !i.Mode().IsRegular() || i.Mode().Perm()&0022 != 0 || i.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != mode || st.Nlink != 1 {
		return "", errors.New("unsafe host policy file")
	}
	if max == 0 {
		if i.Mode().Perm()&0111 == 0 {
			return "", errors.New("helper not executable")
		}
		return "", nil
	}
	if i.Size() < 0 || i.Size() > max {
		return "", errors.New("host policy size exceeded")
	}
	f, e := regularinput.Open(r, b, i)
	if e != nil {
		return "", e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(raw)) != i.Size() {
		return "", errors.New("host policy changed")
	}
	after, e := r.Lstat(b)
	if e != nil || !os.SameFile(i, after) || !i.ModTime().Equal(after.ModTime()) || i.Mode() != after.Mode() {
		return "", errors.New("host policy changed")
	}
	return string(raw), nil
}

// RequireDelegatedMapping supports only a fixed nonroot process identity with
// two single-ID mappings. Root-owned local subid files are the authority;
// alternate NSS allocation backends are not inferred or modified.
func RequireDelegatedMapping(s *Spec) error {
	if s == nil || s.Process == nil || s.Linux == nil || s.Process.User.UID != delegatedGuestID || s.Process.User.GID != delegatedGuestID {
		return errors.New("fixed nonroot guest UID/GID 1000 required")
	}
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	u, e := exactDelegatedMaps(s.Linux.UIDMappings, uid)
	if e != nil {
		return e
	}
	g, e := exactDelegatedMaps(s.Linux.GIDMappings, gid)
	if e != nil {
		return e
	}
	// Do not treat a misconfigured subordinate allocation that resolves to
	// an existing account/group as a safe delegated workload identity.
	var unknownUID user.UnknownUserIdError
	if _, err := user.LookupId(strconv.FormatUint(uint64(u), 10)); err == nil || !errors.As(err, &unknownUID) {
		return errors.New("subordinate UID account absence not verified")
	}
	var unknownGID user.UnknownGroupIdError
	if _, err := user.LookupGroupId(strconv.FormatUint(uint64(g), 10)); err == nil || !errors.As(err, &unknownGID) {
		return errors.New("subordinate GID group absence not verified")
	}
	me, e := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if e != nil || me.Username == "" {
		return errors.New("caller identity unavailable")
	}
	nss, e := trustedHostFile("/etc/nsswitch.conf", 0, 1<<20)
	if e != nil {
		return e
	}
	for _, line := range strings.Split(nss, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "subid:") && strings.TrimSpace(strings.SplitN(line, ":", 2)[1]) != "files" {
			return errors.New("alternate subid backend unsupported")
		}
	}
	for _, item := range []struct {
		file, helper string
		id           uint32
	}{{"/etc/subuid", "/usr/bin/newuidmap", u}, {"/etc/subgid", "/usr/bin/newgidmap", g}} {
		raw, e := trustedHostFile(item.file, 0, 1<<20)
		if e != nil {
			return e
		}
		if e = allocated(raw, me.Username, uid, item.id); e != nil {
			return e
		}
		if _, e = trustedHostFile(item.helper, os.ModeSetuid, 0); e != nil {
			return e
		}
	}
	return nil
}

// ValidateDelegatedLayout permits only a caller-owned private bundle with
// guest-readable read-only regular files and traversable non-writable-for-
// guest directories. No links, special files or mount crossings.
func ValidateDelegatedLayout(ctx context.Context, dir string, s *Spec) error {
	if ctx == nil || s == nil || s.Root == nil || s.Root.Path != "rootfs" || !s.Root.Readonly || s.Process == nil || len(s.Process.Args) == 0 {
		return errors.New("restricted rootfs required")
	}
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return errors.New("absolute private bundle required")
	}
	real, e := filepath.EvalSymlinks(dir)
	if e != nil || real != dir {
		return errors.New("symlinked bundle")
	}
	parent, e := os.Lstat(filepath.Dir(dir))
	if e != nil {
		return e
	}
	if _, e = snapshotInfo(parent, true, 0); e != nil || parent.Mode().Perm() != 0700 {
		return errors.New("private parent required")
	}
	before, e := os.Lstat(dir)
	if e != nil {
		return e
	}
	if _, e = snapshotInfo(before, true, 0); e != nil || before.Mode().Perm() != 0700 {
		return errors.New("private bundle required")
	}
	r, e := os.OpenRoot(dir)
	if e != nil {
		return e
	}
	defer r.Close()
	opened, e := r.Stat(".")
	if e != nil || !os.SameFile(before, opened) {
		return errors.New("bundle root changed")
	}
	config, e := r.Lstat("config.json")
	if e != nil {
		return e
	}
	if _, e = snapshotInfo(config, false, 0); e != nil || config.Mode().Perm() != 0600 && config.Mode().Perm() != 0400 {
		return errors.New("private config required")
	}
	top, e := r.Open(".")
	if e != nil {
		return e
	}
	names, e := top.Readdirnames(3)
	top.Close()
	if e != nil && e != io.EOF {
		return e
	}
	if len(names) != 2 {
		return errors.New("only config and rootfs allowed")
	}
	guest, e := r.OpenRoot("rootfs")
	if e != nil {
		return e
	}
	defer guest.Close()
	mount, e := rootMountID(r)
	if e != nil {
		return e
	}
	gm, e := rootMountID(guest)
	if e != nil || gm != mount {
		return errors.New("rootfs mount boundary")
	}
	count := 0
	var bytes int64
	var walk func(string) error
	walk = func(name string) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		count++
		if count > MaxSnapshotFiles {
			return errors.New("delegated bundle entry quota exceeded")
		}
		i, e := guest.Lstat(name)
		if e != nil {
			return e
		}
		if _, e = snapshotInfo(i, i.IsDir(), 0); e != nil {
			return e
		}
		if i.IsDir() {
			if i.Mode().Perm() != 0755 {
				return errors.New("delegated guest directories require 0755")
			}
			f, e := guest.Open(name)
			if e != nil {
				return e
			}
			actual, e := f.Stat()
			if e != nil || !os.SameFile(i, actual) {
				f.Close()
				return errors.New("directory changed")
			}
			m, e := fileMountID(f)
			if e != nil || m != mount {
				f.Close()
				return errors.New("directory mount boundary")
			}
			entries, e := f.Readdirnames(MaxSnapshotFiles + 1)
			f.Close()
			if e != nil && e != io.EOF {
				return e
			}
			if len(entries) > MaxSnapshotFiles {
				return errors.New("directory quota exceeded")
			}
			for _, n := range entries {
				if e = walk(filepath.Join(name, n)); e != nil {
					return e
				}
			}
		} else {
			if i.Mode().Perm() != 0444 && i.Mode().Perm() != 0555 {
				return errors.New("delegated guest files require 0444/0555")
			}
			if i.Size() < 0 || i.Size() > MaxSnapshotBytes-bytes {
				return errors.New("delegated bundle byte quota exceeded")
			}
			bytes += i.Size()
			f, e := regularinput.Open(guest, name, i)
			if e != nil {
				return e
			}
			m, e := fileMountID(f)
			f.Close()
			if e != nil || m != mount {
				return errors.New("file mount boundary")
			}
		}
		after, e := guest.Lstat(name)
		if e != nil || !os.SameFile(i, after) || i.Mode() != after.Mode() || i.Size() != after.Size() || !i.ModTime().Equal(after.ModTime()) {
			return errors.New("delegated tree changed")
		}
		return nil
	}
	if e = walk("."); e != nil {
		return e
	}
	if e = ValidateRuntimeLayout(dir, s); e != nil {
		return e
	}
	if !HasExplicitGuestExecveAllowance(s) {
		return errors.New("explicit execve allowance required")
	}
	exe := strings.TrimPrefix(s.Process.Args[0], "/")
	i, e := guest.Lstat(exe)
	if e != nil || i.Mode().Perm() != 0555 {
		return errors.New("static guest executable mode required")
	}
	f, e := regularinput.Open(guest, exe, i)
	if e != nil {
		return e
	}
	defer f.Close()
	parsed, e := elf.NewFile(f)
	if e != nil {
		return errors.New("static ELF required")
	}
	defer parsed.Close()
	if parsed.Type != elf.ET_EXEC || parsed.Machine != elf.EM_X86_64 || parsed.Class != elf.ELFCLASS64 {
		return errors.New("native static x86-64 ELF required")
	}
	for _, p := range parsed.Progs {
		if p.Type == elf.PT_INTERP || p.Type == elf.PT_DYNAMIC {
			return errors.New("dynamic ELF unsupported")
		}
	}
	after, e := os.Lstat(dir)
	if e != nil || !os.SameFile(before, after) {
		return errors.New("bundle identity changed")
	}
	return ctx.Err()
}
func PreflightDelegated(ctx context.Context, dir string) (DelegatedPreflight, error) {
	var r DelegatedPreflight
	r.ReadOnly = true
	s, e := Load(dir)
	if e != nil {
		return r, errors.New("restricted OCI policy rejected")
	}
	if e = RequireDelegatedMapping(s); e != nil {
		return r, fmt.Errorf("delegated mapping preflight: %w", e)
	}
	r.MappingVerified = true
	r.HelpersVerified = true
	r.GuestNonRoot = true
	if e = ValidateDelegatedLayout(ctx, dir, s); e != nil {
		return r, fmt.Errorf("delegated layout preflight: %w", e)
	}
	r.LayoutVerified = true
	return r, nil
}
