package ociimage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// StagedLayer is inert, private data from one strictly verified layer.
// Archive executable bits, owners, xattrs, links and whiteouts are not applied.
// Never treat this directory as an admitted executable OCI rootfs.
type StagedLayer struct {
	WhiteoutsApplied         int         `json:"whiteoutsApplied,omitempty"`
	Directory                string      `json:"directory"`
	Layer                    LayerReport `json:"layer"`
	DigestMatches            bool        `json:"digestMatches"`
	DescriptorSizeVerified   bool        `json:"descriptorSizeVerified"`
	SourceProvenanceVerified bool        `json:"sourceProvenanceVerified"`
	ExecutionAuthorized      bool        `json:"executionAuthorized"`
}

// StageLayer writes a fresh directory under an existing user-private parent.
// The same parser and digest stream validate all staged content. No prior
// directory is replaced. Failure removes only the newly created staging tree.
// This is not protection against hostile same-UID processes or hard disk quota.
func StageLayer(ctx context.Context, input io.Reader, media, digest string, size int64, parent string) (report StagedLayer, err error) {
	return StageLayers(ctx, []LayerInput{{Reader: input, Media: media, Digest: digest, Size: size}}, parent)
}

type LayerInput struct {
	Reader io.Reader
	Media  string
	Digest string
	Size   int64
	DiffID string
}

// StageLayers builds an inert ordered overlay from at most eight pinned layers.
// Regular-file replacement is supported only within this fresh private tree.
// Whiteouts, type changes and links remain prohibited; no executable rootfs.
func StageLayers(ctx context.Context, layers []LayerInput, parent string) (report StagedLayer, err error) {
	return stageLayers(ctx, layers, parent, false)
}

// StageLayersWithWhiteouts supports ordered OCI removal markers only for
// seekable, digest-pinned input. It verifies each whole layer twice; all
// effects are restricted to the newly allocated inert tree.
func StageLayersWithWhiteouts(ctx context.Context, layers []LayerInput, parent string) (StagedLayer, error) {
	return stageLayers(ctx, layers, parent, true)
}
func stageLayers(ctx context.Context, layers []LayerInput, parent string, whiteouts bool) (report StagedLayer, err error) {

	if ctx == nil || len(layers) == 0 || len(layers) > 8 {
		return report, errors.New("one to eight layers required")
	}
	for _, l := range layers {
		if l.Reader == nil || !digestPattern.MatchString(l.Digest) || l.Size <= 0 || l.Size > MaxLayerCompressedBytes || (l.DiffID != "" && !digestPattern.MatchString(l.DiffID)) {
			return report, errors.New("invalid layer descriptor")
		}
	}

	if err = ctx.Err(); err != nil {
		return report, err
	}
	if os.Geteuid() == 0 || !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return report, errors.New("nonroot private absolute staging parent required")
	}
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return report, errors.New("symlinked staging parent unsupported")
	}
	before, e := os.Lstat(parent)
	if e != nil {
		return report, e
	}
	if e = privateStoreInfo(before); e != nil {
		return report, e
	}
	base, e := os.OpenRoot(parent)
	if e != nil {
		return report, e
	}
	defer base.Close()
	actual, e := base.Stat(".")
	if e != nil || !os.SameFile(before, actual) {
		return report, errors.New("staging parent changed")
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return report, e
	}
	name := "layer-" + hex.EncodeToString(nonce[:])
	if e = base.Mkdir(name, 0700); e != nil {
		return report, e
	}
	stageInfo, e := base.Lstat(name)
	if e != nil {
		return report, e
	}
	if e = privateStoreInfo(stageInfo); e != nil {
		return report, e
	}
	committed := false
	defer func() {
		if !committed {
			observed, check := base.Lstat(name)
			if check != nil || !os.SameFile(stageInfo, observed) {
				err = errors.Join(err, errors.New("staging identity changed; manual review required"))
				return
			}
			if cleanup := base.RemoveAll(name); cleanup != nil {
				err = errors.Join(err, errors.New("private staging cleanup failed; operator review required"))
			}
		}
	}()
	root, e := base.OpenRoot(name)
	if e != nil {
		return report, e
	}
	defer root.Close()
	dirs := map[string]bool{".": true}
	dirIDs := map[string]os.FileInfo{".": stageInfo}
	markerCount := 0
	if whiteouts {
		opened, e := root.Stat(".")
		if e != nil || !os.SameFile(stageInfo, opened) {
			return report, errors.New("staging root changed")
		}
	}

	files := map[string]os.FileInfo{}
	var mkdir func(string) error
	mkdir = func(n string) error {
		if dirs[n] {
			return nil
		}
		if len(dirs)+len(files) >= MaxLayerEntries {
			return errors.New("staging directory quota exceeded")
		}
		if e := mkdir(path.Dir(n)); e != nil {
			return e
		}
		if len(dirs)+len(files) >= MaxLayerEntries {
			return errors.New("staging directory quota exceeded")
		}
		if e := root.Mkdir(n, 0700); e != nil {
			return e
		}
		dirs[n] = true
		if whiteouts {
			info, e := root.Lstat(n)
			if e != nil || privateStoreInfo(info) != nil {
				return errors.New("unsafe staged directory")
			}
			dirIDs[n] = info
		}
		return nil
	}
	var aggregate LayerReport
	var payload int64
	for _, l := range layers {
		var marker func(string) error
		if whiteouts {
			markers, e := verifiedWhiteouts(ctx, l)
			if e != nil {
				return report, e
			}
			if len(markers) > MaxLayerEntries-markerCount {
				return report, errors.New("aggregate whiteout quota exceeded")
			}
			if e = applyWhiteouts(ctx, root, dirs, dirIDs, files, markers); e != nil {
				return report, e
			}
			markerCount += len(markers)
			marker = func(n string) error { _, e := parseWhiteout(n); return e }
		}
		tracked := &trackedLayerReader{input: l.Reader, hash: sha256.New()}
		decoded := sha256.New()
		layer, e := inspectLayerPolicy(ctx, tracked, l.Media, func(n string, dir bool, length int64, r io.Reader) error {
			if dir {
				if files[n] != nil {
					return errors.New("layer type replacement unsupported")
				}
				return mkdir(n)
			}
			if e := mkdir(path.Dir(n)); e != nil {
				return e
			}
			if dirs[n] {
				return errors.New("layer type replacement unsupported")
			}
			if length > MaxLayerUnpackedBytes-payload {
				return errors.New("aggregate layer payload quota exceeded")
			}
			payload += length
			if len(files)+len(dirs) >= MaxLayerEntries && files[n] == nil {
				return errors.New("aggregate layer entry quota exceeded")
			}
			return writeStagedFile(root, files, n, length, r)

		}, decoded, marker)
		if e != nil {
			return report, e
		}
		if tracked.bytes != l.Size || "sha256:"+hex.EncodeToString(tracked.hash.Sum(nil)) != l.Digest {
			return report, errors.New("staged OCI layer descriptor mismatch")
		}
		if l.DiffID != "" && "sha256:"+hex.EncodeToString(decoded.Sum(nil)) != l.DiffID {
			return report, errors.New("uncompressed OCI layer diff ID mismatch")
		}
		aggregate.FileCount += layer.FileCount
		aggregate.DirectoryCount += layer.DirectoryCount
		aggregate.UnpackedBytes += layer.UnpackedBytes
		if aggregate.FileCount+aggregate.DirectoryCount > MaxLayerEntries || len(files)+len(dirs) > MaxLayerEntries {
			return report, errors.New("aggregate layer entry quota exceeded")
		}
	}
	if e = ctx.Err(); e != nil {
		return report, e
	}
	current, e := os.Lstat(parent)
	if e != nil || !os.SameFile(before, current) {
		return report, errors.New("staging parent changed before completion")
	}
	observed, e := base.Lstat(name)
	if e != nil || !os.SameFile(stageInfo, observed) {
		return report, errors.New("staging identity changed before completion")
	}
	// Sync directory entries bottom-up before reporting the tree.
	names := make([]string, 0, len(dirs))
	for n := range dirs {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "." {
			return false
		}
		if names[j] == "." {
			return true
		}
		di, dj := strings.Count(names[i], "/"), strings.Count(names[j], "/")
		if di != dj {
			return di > dj
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		f, e := root.Open(n)
		if e != nil {
			return report, e
		}
		e = errors.Join(f.Sync(), f.Close())
		if e != nil {
			return report, e
		}
	}
	f, e := base.Open(".")
	if e != nil {
		return report, e
	}
	if e = errors.Join(f.Sync(), f.Close()); e != nil {
		return report, e
	}
	committed = true
	return StagedLayer{WhiteoutsApplied: markerCount, Directory: filepath.Join(parent, name), Layer: aggregate, DigestMatches: true, DescriptorSizeVerified: true}, nil
}

// Never truncate an existing inode: verify its identity and write an exclusive
// sibling temporary file, then rename over the staged path. A same-UID actor
// can still race path changes; this is not an adversarial filesystem boundary.
func writeStagedFile(root *os.Root, files map[string]os.FileInfo, n string, length int64, r io.Reader) error {
	previous := files[n]
	if previous != nil {
		info, e := root.Lstat(n)
		if e != nil || !os.SameFile(previous, info) || !safeStagedFile(info) {
			return errors.New("unsafe staged replacement")
		}
	}
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return e
	}
	tmp := path.Join(path.Dir(n), ".gc-layer-"+hex.EncodeToString(nonce[:]))
	f, e := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer root.Remove(tmp)
	written, e := io.CopyN(f, r, length)
	if e == nil && written != length {
		e = io.ErrUnexpectedEOF
	}
	if e == nil {
		e = f.Sync()
	}
	info, statErr := f.Stat()
	e = errors.Join(e, statErr, f.Close())
	if e != nil {
		return e
	}
	if !safeStagedFile(info) {
		return errors.New("unsafe staged temporary file")
	}
	if previous != nil {
		current, e := root.Lstat(n)
		if e != nil || !os.SameFile(previous, current) || !safeStagedFile(current) {
			return errors.New("staged replacement identity changed")
		}
		if e = root.Rename(tmp, n); e != nil {
			return e
		}
	} else {
		if e = root.Link(tmp, n); e != nil {
			return e
		}
		if e = root.Remove(tmp); e != nil {
			return e
		}
	}
	actual, e := root.Lstat(n)
	if e != nil || !os.SameFile(info, actual) || !safeStagedFile(actual) {
		return errors.New("staged output identity changed")
	}
	files[n] = actual
	return nil
}
func safeStagedFile(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid()) && st.Nlink == 1
}
