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
	"syscall"
)

// StagedLayer is inert, private data from one strictly verified layer.
// Archive executable bits, owners, xattrs, links and whiteouts are not applied.
// Never treat this directory as an admitted executable OCI rootfs.
type StagedLayer struct {
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
}

// StageLayers builds an inert ordered overlay from at most eight pinned layers.
// Regular-file replacement is supported only within this fresh private tree.
// Whiteouts, type changes and links remain prohibited; no executable rootfs.
func StageLayers(ctx context.Context, layers []LayerInput, parent string) (report StagedLayer, err error) {
	if ctx == nil || len(layers) == 0 || len(layers) > 8 {
		return report, errors.New("one to eight layers required")
	}
	for _, l := range layers {
		if l.Reader == nil || !digestPattern.MatchString(l.Digest) || l.Size <= 0 || l.Size > MaxLayerCompressedBytes {
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
	var mkdir func(string) error
	mkdir = func(n string) error {
		if dirs[n] {
			return nil
		}
		if len(dirs) >= MaxLayerEntries {
			return errors.New("staging directory quota exceeded")
		}
		if e := mkdir(path.Dir(n)); e != nil {
			return e
		}
		if len(dirs) >= MaxLayerEntries {
			return errors.New("staging directory quota exceeded")
		}
		if e := root.Mkdir(n, 0700); e != nil {
			return e
		}
		dirs[n] = true
		return nil
	}
	var aggregate LayerReport
	files := map[string]bool{}
	var payload int64
	for _, l := range layers {
		tracked := &trackedLayerReader{input: l.Reader, hash: sha256.New()}
		layer, e := inspectLayer(ctx, tracked, l.Media, func(n string, dir bool, length int64, r io.Reader) error {
			if dir {
				if files[n] {
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
			if len(files)+len(dirs) >= MaxLayerEntries && !files[n] {
				return errors.New("aggregate layer entry quota exceeded")
			}
			flags := os.O_CREATE | os.O_EXCL | os.O_WRONLY
			if files[n] {
				info, e := root.Lstat(n)
				if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
					return errors.New("unsafe staged replacement")
				}
				// All files belong to this newly created unpublished tree.
				flags = os.O_TRUNC | os.O_WRONLY
			}
			f, e := root.OpenFile(n, flags|syscall.O_NOFOLLOW, 0600)
			files[n] = true
			if e != nil {
				return e
			}
			written, e := io.CopyN(f, r, length)
			if e == nil && written != length {
				e = io.ErrUnexpectedEOF
			}
			if e == nil {
				e = f.Sync()
			}
			return errors.Join(e, f.Close())
		})
		if e != nil {
			return report, e
		}
		if tracked.bytes != l.Size || "sha256:"+hex.EncodeToString(tracked.hash.Sum(nil)) != l.Digest {
			return report, errors.New("staged OCI layer descriptor mismatch")
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
	committed = true
	return StagedLayer{Directory: filepath.Join(parent, name), Layer: aggregate, DigestMatches: true, DescriptorSizeVerified: true}, nil
}
