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
	if ctx == nil || input == nil || !digestPattern.MatchString(digest) || size <= 0 || size > MaxLayerCompressedBytes {
		return report, errors.New("invalid staging descriptor")
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
	tracked := &trackedLayerReader{input: input, hash: sha256.New()}
	layer, e := inspectLayer(ctx, tracked, media, func(n string, dir bool, length int64, r io.Reader) error {
		if dir {
			return mkdir(n)
		}
		if e := mkdir(path.Dir(n)); e != nil {
			return e
		}
		f, e := root.OpenFile(n, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
	if tracked.bytes != size || "sha256:"+hex.EncodeToString(tracked.hash.Sum(nil)) != digest {
		return report, errors.New("staged OCI layer descriptor mismatch")
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
	return StagedLayer{Directory: filepath.Join(parent, name), Layer: layer, DigestMatches: true, DescriptorSizeVerified: true}, nil
}
