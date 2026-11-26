package ociimage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"syscall"
)

type StoreAssemblyReport struct {
	ManifestDigest  string `json:"manifestDigest"`
	ConfigDigest    string `json:"configDigest"`
	LayerCount      int    `json:"layerCount"`
	Architecture    string `json:"architecture"`
	OS              string `json:"os"`
	DiffIDsVerified bool   `json:"diffIdsVerified"`
	StagedLayer
}

func openPinnedStoreBlob(root *os.Root, d ManifestDescriptor) (*os.File, error) {
	if !digestPattern.MatchString(d.Digest) || d.Size <= 0 || d.Size > maxBlobBytes {
		return nil, errors.New("invalid pinned store descriptor")
	}
	name := "blobs/sha256/" + d.Digest[len("sha256:"):]
	info, e := root.Lstat(name)
	if e != nil || !info.Mode().IsRegular() || info.Size() != d.Size || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("missing or unsafe pinned store blob")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return nil, errors.New("unsafe store blob identity")
	}
	return regularinput.Open(root, name, info)
}
func readPinnedMetadata(ctx context.Context, root *os.Root, d ManifestDescriptor) ([]byte, error) {
	if d.Size <= 0 || d.Size > maxMetadataBytes {
		return nil, errors.New("metadata descriptor exceeds bound")
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	f, e := openPinnedStoreBlob(root, d)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return nil, e
	}
	raw, e := io.ReadAll(io.LimitReader(f, d.Size+1))
	if e != nil || int64(len(raw)) != d.Size || fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != d.Digest {
		return nil, errors.New("metadata digest or size mismatch")
	}
	if e = verifyLayoutReadIdentity(root, f, before, "blobs/sha256/"+d.Digest[len("sha256:"):]); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return raw, nil
}
func configDiffIDs(raw []byte, layers int) (string, []string, error) {
	obj, e := decodeObject(raw)
	if e != nil {
		return "", nil, e
	}
	osName, e := stringField(obj, "os")
	if e != nil || osName != "linux" {
		return "", nil, errors.New("only Linux image metadata supported")
	}
	arch, e := stringField(obj, "architecture")
	if e != nil || (arch != "amd64" && arch != "arm64") {
		return "", nil, errors.New("unsupported image architecture")
	}
	rootfs, e := decodeObject(obj["rootfs"])
	if e != nil {
		return "", nil, e
	}
	kind, e := stringField(rootfs, "type")
	if e != nil || kind != "layers" || len(rootfs) != 2 {
		return "", nil, errors.New("unsupported rootfs metadata")
	}
	var ids []string
	if e = json.Unmarshal(rootfs["diff_ids"], &ids); e != nil || len(ids) != layers || ids == nil {
		return "", nil, errors.New("rootfs diff ID count mismatch")
	}
	for _, id := range ids {
		if !digestPattern.MatchString(id) {
			return "", nil, errors.New("invalid diff ID")
		}
	}
	return arch, ids, nil
}

// AssembleManifest validates one explicit local manifest, its exact config and
// every ordered compressed descriptor AND uncompressed diff ID in the same
// staging pass. The result remains inert data, never an executable rootfs.
func (s *Store) AssembleManifest(ctx context.Context, digest string, size int64, parent string) (StoreAssemblyReport, error) {
	var empty StoreAssemblyReport
	if s == nil || ctx == nil {
		return empty, errors.New("store and context required")
	}
	root, e := s.openRoot()
	if e != nil {
		return empty, e
	}
	defer root.Close()
	raw, e := readPinnedMetadata(ctx, root, ManifestDescriptor{MediaManifest, digest, size})
	if e != nil {
		return empty, e
	}
	manifest, e := parsePinnedManifest(raw, digest, size)
	if e != nil {
		return empty, e
	}
	if len(manifest.Layers) == 0 {
		return empty, errors.New("empty image assembly unsupported")
	}
	config, e := readPinnedMetadata(ctx, root, manifest.Config)
	if e != nil {
		return empty, e
	}
	arch, ids, e := configDiffIDs(config, len(manifest.Layers))
	if e != nil {
		return empty, e
	}
	inputs := make([]LayerInput, 0, len(ids))
	for i, d := range manifest.Layers {
		f, e := openPinnedStoreBlob(root, d)
		if e != nil {
			return empty, e
		}
		defer f.Close()
		inputs = append(inputs, LayerInput{Reader: f, Media: d.MediaType, Digest: d.Digest, Size: d.Size, DiffID: ids[i]})
	}
	staged, e := StageLayers(ctx, inputs, parent)
	if e != nil {
		return empty, e
	}
	return StoreAssemblyReport{ManifestDigest: digest, ConfigDigest: manifest.Config.Digest, LayerCount: len(ids), Architecture: arch, OS: "linux", DiffIDsVerified: true, StagedLayer: staged}, nil
}
