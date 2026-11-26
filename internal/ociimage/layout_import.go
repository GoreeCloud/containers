package ociimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/GoreeCloud/containers/internal/regularinput"
	"io"
	"os"
	"path/filepath"
)

type LayoutImportReport struct {
	StoreAssemblyReport
	ImportedBlobs int    `json:"importedBlobs"`
	ReusedBlobs   int    `json:"reusedBlobs"`
	IndexDigest   string `json:"indexDigest"`
}

func layoutBytes(root *os.Root, name string, max int64) ([]byte, error) {
	i, e := root.Lstat(name)
	if e != nil || !i.Mode().IsRegular() || i.Size() <= 0 || i.Size() > max {
		return nil, errors.New("invalid layout metadata")
	}
	f, e := regularinput.Open(root, name, i)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) != i.Size() {
		return nil, errors.New("layout metadata changed")
	}
	if e = verifyLayoutReadIdentity(root, f, i, name); e != nil {
		return nil, e
	}
	return b, nil
}

// ImportLayoutAndStage imports only a uniquely named direct manifest in the
// explicitly pinned index. Every CAS write is independently digest-verified.
// The existing bounded assembly pass verifies every DiffID and tar policy.
func (s *Store) ImportLayoutAndStage(ctx context.Context, layout, indexPin, manifestPin string, size int64, parent string, whiteouts bool) (LayoutImportReport, error) {
	var empty LayoutImportReport
	if s == nil || ctx == nil || !digestPattern.MatchString(indexPin) || !digestPattern.MatchString(manifestPin) || size <= 0 || size > maxMetadataBytes {
		return empty, errors.New("exact index and manifest pins required")
	}
	if !filepath.IsAbs(layout) || filepath.Clean(layout) != layout {
		return empty, errors.New("absolute layout path required")
	}
	real, e := filepath.EvalSymlinks(layout)
	if e != nil || real != layout {
		return empty, errors.New("unsafe layout path")
	}
	before, e := os.Lstat(layout)
	if e != nil || !before.IsDir() {
		return empty, errors.New("layout directory required")
	}
	root, e := os.OpenRoot(layout)
	if e != nil {
		return empty, e
	}
	defer root.Close()
	dirs := map[string]os.FileInfo{".": before}
	for _, name := range []string{"blobs", "blobs/sha256"} {
		i, e := root.Lstat(name)
		if e != nil || !i.IsDir() {
			return empty, errors.New("unsafe blob directory")
		}
		dirs[name] = i
	}
	check := func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		for name, i := range dirs {
			a, e := root.Lstat(name)
			if e != nil || !a.IsDir() || !os.SameFile(i, a) {
				return errors.New("layout directory changed")
			}
			b, e := os.Lstat(filepath.Join(layout, name))
			if e != nil || !b.IsDir() || !os.SameFile(i, b) {
				return errors.New("named layout changed")
			}
		}
		return nil
	}
	if e = check(); e != nil {
		return empty, e
	}
	marker, e := readObject(root, "oci-layout", 4096)
	if e != nil {
		return empty, e
	}
	v, e := stringField(marker, "imageLayoutVersion")
	if e != nil || v != "1.0.0" {
		return empty, errors.New("unsupported layout version")
	}
	index, e := layoutBytes(root, "index.json", maxMetadataBytes)
	if e != nil {
		return empty, e
	}
	if fmt.Sprintf("sha256:%x", sha256.Sum256(index)) != indexPin {
		return empty, errors.New("index digest mismatch")
	}
	obj, e := decodeObject(index)
	if e != nil {
		return empty, e
	}
	if e = schemaTwo(obj); e != nil {
		return empty, e
	}
	if _, ok := obj["mediaType"]; ok {
		m, e := stringField(obj, "mediaType")
		if e != nil || m != MediaIndex {
			return empty, errors.New("invalid layout index media type")
		}
	}
	for _, key := range []string{"artifactType", "subject"} {
		if _, ok := obj[key]; ok {
			return empty, errors.New("artifact index unsupported")
		}
	}
	children, e := descriptors(obj["manifests"])
	if e != nil || len(children) == 0 || len(children) > 32 {
		return empty, errors.New("direct layout manifest list required")
	}
	found := false
	seen := map[string]bool{}
	for _, child := range children {
		d, e := parseDescriptor(child)
		if e != nil || d.Media != MediaManifest || d.Size <= 0 || d.Size > maxMetadataBytes || seen[d.Digest] {
			return empty, errors.New("unsupported or duplicate direct layout descriptor")
		}
		if _, ok := child["artifactType"]; ok {
			return empty, errors.New("artifact descriptor unsupported")
		}
		seen[d.Digest] = true
		if d.Digest == manifestPin {
			if d.Size != size {
				return empty, errors.New("manifest size mismatch")
			}
			found = true
		}
	}
	if !found {
		return empty, errors.New("pinned manifest not in index")
	}
	metadata := func(d ManifestDescriptor) ([]byte, error) {
		b, e := layoutBytes(root, "blobs/sha256/"+d.Digest[7:], maxMetadataBytes)
		if e != nil || int64(len(b)) != d.Size || fmt.Sprintf("sha256:%x", sha256.Sum256(b)) != d.Digest {
			return nil, errors.New("layout metadata pin mismatch")
		}
		return b, check()
	}
	md := ManifestDescriptor{MediaManifest, manifestPin, size}
	manifestRaw, e := metadata(md)
	if e != nil {
		return empty, e
	}
	manifest, e := parsePinnedManifest(manifestRaw, manifestPin, size)
	if e != nil {
		return empty, e
	}
	if len(manifest.Layers) == 0 {
		return empty, errors.New("empty layout manifest unsupported")
	}
	configRaw, e := metadata(manifest.Config)
	if e != nil {
		return empty, e
	}
	if _, _, e = configDiffIDs(configRaw, len(manifest.Layers)); e != nil {
		return empty, e
	}
	audit, e := s.Audit(ctx)
	if e != nil || !audit.Verified || audit.OperatorReviewRequired {
		return empty, errors.New("existing store requires review")
	}
	dest, e := s.openRoot()
	if e != nil {
		return empty, e
	}
	defer dest.Close()
	r := LayoutImportReport{IndexDigest: indexPin}
	acquired := map[string]ManifestDescriptor{}
	acquire := func(d ManifestDescriptor, raw []byte) error {
		if old, ok := acquired[d.Digest]; ok {
			if old != d {
				return errors.New("conflicting descriptors")
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		// Verify the source even if an identical destination object exists.
		var source io.Reader
		var f *os.File
		var i os.FileInfo
		var name string
		var e error
		if raw != nil {
			source = bytes.NewReader(raw)
		} else {
			name = "blobs/sha256/" + d.Digest[7:]
			i, e = root.Lstat(name)
			if e != nil || !i.Mode().IsRegular() || i.Size() != d.Size {
				return errors.New("missing or unsafe source layer")
			}
			f, e = regularinput.Open(root, name, i)
			if e != nil {
				return e
			}
			defer f.Close()
			source = f

		}
		_, e = dest.Lstat("blobs/sha256/" + d.Digest[7:])
		exists := e == nil
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if _, e = s.PutVerified(ctx, source, d.Digest, d.Size); e != nil {
			return e
		}
		if f != nil {
			if e = verifyLayoutReadIdentity(root, f, i, name); e != nil {
				return e
			}
		}
		if exists {
			r.ReusedBlobs++
		} else {
			r.ImportedBlobs++
		}
		acquired[d.Digest] = d
		return check()
	}
	if e = acquire(md, manifestRaw); e != nil {
		return empty, e
	}
	if e = acquire(manifest.Config, configRaw); e != nil {
		return empty, e
	}
	for _, d := range manifest.Layers {
		if d.Size > MaxLayerCompressedBytes {
			return empty, errors.New("layer exceeds assembly bound")
		}
		if e = acquire(d, nil); e != nil {
			return empty, e
		}
	}
	if e = check(); e != nil {
		return empty, e
	}
	staged, e := s.assembleManifest(ctx, manifestPin, size, parent, whiteouts)
	if e != nil {
		return empty, e
	}
	r.StoreAssemblyReport = staged
	return r, nil
}
