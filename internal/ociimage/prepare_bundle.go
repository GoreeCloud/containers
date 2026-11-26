package ociimage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/GoreeCloud/containers/internal/bundle"
	"github.com/GoreeCloud/containers/internal/regularinput"
)

type PreparedBundle struct {
	DelegatedGuest           bool   `json:"delegatedGuest,omitempty"`
	WhiteoutsApplied         int    `json:"whiteoutsApplied,omitempty"`
	Directory                string `json:"directory"`
	ManifestDigest           string `json:"manifestDigest"`
	ConfigSHA256             string `json:"configSHA256"`
	EntrypointSHA256         string `json:"entrypointSHA256"`
	DiffIDsVerified          bool   `json:"diffIdsVerified"`
	LayoutVerified           bool   `json:"layoutVerified"`
	SourceProvenanceVerified bool   `json:"sourceProvenanceVerified"`
	ExecutionAuthorized      bool   `json:"executionAuthorized"`
	Immutable                bool   `json:"immutable"`
}

func pinnedLocalConfig(ctx context.Context, name, pin string) ([]byte, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name || !digestPattern.MatchString(pin) {
		return nil, errors.New("absolute pinned config required")
	}
	parent := filepath.Dir(name)
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return nil, errors.New("unsafe config parent")
	}
	r, e := os.OpenRoot(parent)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	n := filepath.Base(name)
	fi, e := r.Lstat(n)
	if e != nil || !fi.Mode().IsRegular() || fi.Size() <= 0 || fi.Size() > bundle.MaxConfigBytes {
		return nil, errors.New("invalid local config")
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || fi.Mode().Perm()&0022 != 0 {
		return nil, errors.New("unsafe local config ownership")
	}
	f, e := regularinput.Open(r, n, fi)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, bundle.MaxConfigBytes+1))
	if e != nil || int64(len(raw)) != fi.Size() || fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != pin {
		return nil, errors.New("local config pin mismatch")
	}
	if e = verifyLayoutReadIdentity(r, f, fi, n); e != nil {
		return nil, e
	}
	return raw, ctx.Err()
}

// PrepareBundle materializes a restricted executable OCI bundle from inert
// pinned image bytes and a separately reviewed local config and entrypoint
// digest. It never interprets image configuration as runtime authorization.
func (s *Store) PrepareBundle(ctx context.Context, digest string, size int64, config, configPin, entryPin, parent string) (report PreparedBundle, err error) {
	return s.prepareBundle(ctx, digest, size, config, configPin, entryPin, parent, false, false)
}
func (s *Store) PrepareBundleWithWhiteouts(ctx context.Context, digest string, size int64, config, configPin, entryPin, parent string) (PreparedBundle, error) {
	return s.prepareBundle(ctx, digest, size, config, configPin, entryPin, parent, true, false)
}

// PrepareDelegatedBundle uses the same pinned static-image pipeline but a
// separately selected guest mapping and guest-readable private rootfs.
func (s *Store) PrepareDelegatedBundle(ctx context.Context, digest string, size int64, config, configPin, entryPin, parent string, whiteouts bool) (PreparedBundle, error) {
	return s.prepareBundle(ctx, digest, size, config, configPin, entryPin, parent, whiteouts, true)
}
func (s *Store) prepareBundle(ctx context.Context, digest string, size int64, config, configPin, entryPin, parent string, whiteouts, delegated bool) (report PreparedBundle, err error) {

	if ctx == nil || s == nil || os.Geteuid() == 0 || !digestPattern.MatchString(entryPin) {
		return report, errors.New("nonroot pinned preparation required")
	}
	raw, e := pinnedLocalConfig(ctx, config, configPin)
	if e != nil {
		return report, e
	}
	if !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return report, errors.New("absolute private parent required")
	}
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return report, errors.New("unsafe parent")
	}
	before, e := os.Lstat(parent)
	if e != nil || privateStoreInfo(before) != nil {
		return report, errors.New("unsafe preparation parent")
	}
	base, e := os.OpenRoot(parent)
	if e != nil {
		return report, e
	}
	defer base.Close()
	opened, e := base.Stat(".")
	if e != nil || !os.SameFile(before, opened) {
		return report, errors.New("parent changed")
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return report, e
	}
	name := fmt.Sprintf("prepared-%x", nonce[:])
	if e = base.Mkdir(name, 0700); e != nil {
		return report, e
	}
	identity, e := base.Lstat(name)
	if e != nil {
		return report, e
	}
	keep := false
	defer func() {
		if !keep {
			current, e := base.Lstat(name)
			if e != nil || !os.SameFile(identity, current) {
				err = errors.Join(err, errors.New("preparation tree identity changed; review required"))
				return
			}
			err = errors.Join(err, base.RemoveAll(name))
		}
	}()
	root, e := base.OpenRoot(name)
	if e != nil {
		return report, e
	}
	defer root.Close()
	opened, e = root.Stat(".")
	if e != nil || !os.SameFile(identity, opened) {
		return report, errors.New("prepared directory changed")
	}
	dir := filepath.Join(parent, name)
	staged, e := s.assembleManifest(ctx, digest, size, dir, whiteouts)
	if e != nil {
		return report, e
	}
	if staged.Architecture != runtime.GOARCH {
		return report, errors.New("image architecture differs from local host")
	}
	stageName := filepath.Base(staged.Directory)
	if filepath.Dir(staged.Directory) != dir {
		return report, errors.New("staging escaped preparation")
	}
	if e = root.Rename(stageName, "rootfs"); e != nil {
		return report, e
	}
	f, e := root.OpenFile("config.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0400)
	if e != nil {
		return report, e
	}
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e == nil {
		e = f.Sync()
	}
	if e = errors.Join(e, f.Close()); e != nil {
		return report, e
	}
	spec, e := bundle.Load(dir)
	if e != nil {
		return report, e
	}
	if spec.Root.Path != "rootfs" {
		return report, errors.New("prepared configuration root must be rootfs")
	}
	if delegated {
		if e = bundle.RequireDelegatedMapping(spec); e != nil {
			return report, e
		}
	} else if e = bundle.RequireSingleCallerMapping(spec, uint32(os.Geteuid()), uint32(os.Getegid())); e != nil {
		return report, e
	}
	if !bundle.HasExplicitGuestExecveAllowance(spec) {
		return report, errors.New("explicit guest execve allowance required")
	}
	guest, e := root.OpenRoot("rootfs")
	if e != nil {
		return report, e
	}
	defer guest.Close()
	for _, n := range []string{"dev", "proc", "dev/pts"} {
		if e = guest.Mkdir(n, 0700); e != nil && !os.IsExist(e) {
			return report, e
		}
		info, e := guest.Lstat(n)
		if e != nil || privateStoreInfo(info) != nil {
			return report, errors.New("invalid guest mountpoint")
		}
	}
	exe := strings.TrimPrefix(spec.Process.Args[0], "/")
	if !filepath.IsLocal(exe) || filepath.Clean(exe) != exe {
		return report, errors.New("invalid guest entrypoint")
	}
	fi, e := guest.Lstat(exe)
	if e != nil || !safeStagedFile(fi) || fi.Size() <= 0 || fi.Size() > 64<<20 {
		return report, errors.New("entrypoint is not a bounded staged regular file")
	}
	ef, e := regularinput.Open(guest, exe, fi)
	if e != nil {
		return report, e
	}
	defer ef.Close()
	h := sha256.New()
	count, e := io.Copy(h, io.LimitReader(ef, fi.Size()+1))
	if e != nil || count != fi.Size() || fmt.Sprintf("sha256:%x", h.Sum(nil)) != entryPin {
		return report, errors.New("entrypoint digest mismatch")
	}
	if e = verifyLayoutReadIdentity(guest, ef, fi, exe); e != nil {
		return report, e
	}
	if _, e = ef.Seek(0, io.SeekStart); e != nil {
		return report, e
	}
	elfFile, e := elf.NewFile(ef)
	if e != nil {
		return report, errors.New("only reviewed static ELF entrypoints supported")
	}
	defer elfFile.Close()
	if elfFile.Type != elf.ET_EXEC || elfFile.Class != elf.ELFCLASS64 || (runtime.GOARCH == "amd64" && elfFile.Machine != elf.EM_X86_64) || (runtime.GOARCH == "arm64" && elfFile.Machine != elf.EM_AARCH64) {
		return report, errors.New("unsupported entrypoint ELF")
	}
	for _, p := range elfFile.Progs {
		if p.Type == elf.PT_INTERP || p.Type == elf.PT_DYNAMIC {
			return report, errors.New("dynamic entrypoint unsupported")
		}
	}
	if e = ef.Chmod(0500); e != nil {
		return report, e
	}
	if e = ef.Sync(); e != nil {
		return report, e
	}
	if delegated {
		if e = normalizeDelegatedStaging(ctx, guest, exe); e != nil {
			return report, e
		}
		if e = bundle.ValidateDelegatedLayout(ctx, dir, spec); e != nil {
			return report, e
		}
	}
	if e = bundle.ValidateRuntimeLayout(dir, spec); e != nil {
		return report, e
	}
	for _, n := range []string{"dev/pts", "dev", "proc", "."} {
		f, e := guest.Open(n)
		if e != nil {
			return report, e
		}
		if e = errors.Join(f.Sync(), f.Close()); e != nil {
			return report, e
		}
	}
	for _, r := range []*os.Root{root, base} {
		f, e := r.Open(".")
		if e != nil {
			return report, e
		}
		if e = errors.Join(f.Sync(), f.Close()); e != nil {
			return report, e
		}
	}
	current, e := os.Lstat(parent)
	if e != nil || !os.SameFile(before, current) {
		return report, errors.New("parent changed")
	}
	current, e = base.Lstat(name)
	if e != nil || !os.SameFile(identity, current) {
		return report, errors.New("prepared root changed")
	}
	if e = ctx.Err(); e != nil {
		return report, e
	}
	keep = true
	return PreparedBundle{DelegatedGuest: delegated, WhiteoutsApplied: staged.WhiteoutsApplied, Directory: dir, ManifestDigest: digest, ConfigSHA256: configPin, EntrypointSHA256: entryPin, DiffIDsVerified: true, LayoutVerified: true}, nil
}
