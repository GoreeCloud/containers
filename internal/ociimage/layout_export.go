package ociimage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type LayoutExportReport struct {
	Directory                string `json:"directory"`
	IndexDigest              string `json:"indexDigest"`
	ManifestDigest           string `json:"manifestDigest"`
	ManifestSize             int64  `json:"manifestSize"`
	VerifiedBlobs            int    `json:"verifiedBlobs"`
	VerifiedBytes            int64  `json:"verifiedBytes"`
	DiffIDsVerified          bool   `json:"diffIdsVerified"`
	ArchiveSubsetVerified    bool   `json:"archiveSubsetVerified"`
	SourceProvenanceVerified bool   `json:"sourceProvenanceVerified"`
	ExecutionAuthorized      bool   `json:"executionAuthorized"`
}

func exportSyncDir(root *os.Root, name string) error {
	f, e := root.Open(name)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}

// ExportLayout copies only the explicitly pinned manifest graph into a fresh
// private OCI layout. It checks archive framing and DiffIDs, not overlay
// application semantics or image authorization. No source writes.
func (s *Store) ExportLayout(ctx context.Context, digest string, size int64, parent string, whiteouts bool) (report LayoutExportReport, err error) {
	if s == nil || ctx == nil || !digestPattern.MatchString(digest) || size <= 0 || size > maxMetadataBytes || os.Geteuid() == 0 || !filepath.IsAbs(parent) || filepath.Clean(parent) != parent {
		return report, errors.New("private export parent and exact manifest pin required")
	}
	real, e := filepath.EvalSymlinks(parent)
	if e != nil || real != parent {
		return report, errors.New("unsafe export parent")
	}
	pi, e := os.Lstat(parent)
	if e != nil || privateStoreInfo(pi) != nil {
		return report, errors.New("private export parent required")
	}
	base, e := os.OpenRoot(parent)
	if e != nil {
		return report, e
	}
	defer base.Close()
	p, e := base.Stat(".")
	if e != nil || !os.SameFile(pi, p) {
		return report, errors.New("export parent changed")
	}
	src, e := s.openRoot()
	if e != nil {
		return report, e
	}
	defer src.Close()
	md := ManifestDescriptor{MediaManifest, digest, size}
	raw, e := readPinnedMetadata(ctx, src, md)
	if e != nil {
		return report, e
	}
	m, e := parsePinnedManifest(raw, digest, size)
	if e != nil || len(m.Layers) == 0 {
		return report, errors.New("supported nonempty manifest required")
	}
	conf, e := readPinnedMetadata(ctx, src, m.Config)
	if e != nil {
		return report, e
	}
	_, ids, e := configDiffIDs(conf, len(m.Layers))
	if e != nil {
		return report, e
	}
	// Do not create exports under the content store or use a directory above it.
	for _, pair := range [][2]string{{s.root, parent}, {parent, s.root}} {
		if rel, e := filepath.Rel(pair[0], pair[1]); e == nil && (rel == "." || filepath.IsLocal(rel)) {
			return report, errors.New("separate export parent required")
		}
	}
	audit, e := s.Audit(ctx)
	if e != nil || !audit.Verified || audit.OperatorReviewRequired {
		return report, errors.New("existing store requires review")
	}
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return report, e
	}
	name := fmt.Sprintf("oci-layout-%x", nonce)
	if e = base.Mkdir(name, 0700); e != nil {
		return report, e
	}
	fi, e := base.Lstat(name)
	if e != nil || privateStoreInfo(fi) != nil {
		return report, errors.New("new private export directory unavailable")
	}
	root, e := base.OpenRoot(name)
	if e != nil {
		return report, e
	}
	defer root.Close()
	// An incomplete private export is deliberately retained for review, never
	// reported as a completed layout. Publish index.json only after all copies.
	report.Directory = filepath.Join(parent, name)
	dirs := map[string]os.FileInfo{}
	check := func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		a, e := os.Lstat(parent)
		if e != nil || !os.SameFile(pi, a) || privateStoreInfo(a) != nil {
			return errors.New("export parent replaced")
		}
		b, e := base.Lstat(name)
		if e != nil || !os.SameFile(fi, b) || privateStoreInfo(b) != nil {
			return errors.New("export directory replaced")
		}
		c, e := root.Stat(".")
		if e != nil || !os.SameFile(fi, c) {
			return errors.New("export root anchor mismatch")
		}
		for n, expected := range dirs {
			observed, e := root.Lstat(n)
			if e != nil || privateStoreInfo(observed) != nil || !os.SameFile(expected, observed) {
				return errors.New("export blob directory changed")
			}
		}
		return nil
	}
	if e = check(); e != nil {
		return report, e
	}
	if e = root.Mkdir("blobs", 0700); e != nil {
		return report, e
	}
	if e = root.Mkdir("blobs/sha256", 0700); e != nil {
		return report, e
	}
	for _, n := range []string{"blobs", "blobs/sha256"} {
		i, e := root.Lstat(n)
		if e != nil || privateStoreInfo(i) != nil {
			return report, errors.New("unsafe export blob directory")
		}
		dirs[n] = i
	}
	write := func(name string, b []byte) error {
		if e := check(); e != nil {
			return e
		}
		f, e := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		return errors.Join(e, f.Close())
	}
	seen := map[string]ManifestDescriptor{}
	copied := int64(0)
	copyBlob := func(d ManifestDescriptor) error {
		if old, ok := seen[d.Digest]; ok {
			if old != d {
				return errors.New("conflicting export descriptor")
			}
			return nil
		}
		if e := check(); e != nil {
			return e
		}
		f, e := openPinnedStoreBlob(src, d)
		if e != nil {
			return e
		}
		defer f.Close()
		before, e := f.Stat()
		if e != nil {
			return e
		}
		n := "blobs/sha256/" + d.Digest[7:]
		dst, e := root.OpenFile(n, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		h := sha256.New()
		count, e := io.Copy(io.MultiWriter(dst, h), io.LimitReader(&layerContextReader{ctx: ctx, reader: f}, d.Size+1))
		if e == nil && (count != d.Size || fmt.Sprintf("sha256:%x", h.Sum(nil)) != d.Digest) {
			e = errors.New("export blob digest or size mismatch")
		}
		if e == nil {
			e = dst.Sync()
		}
		if e = errors.Join(e, dst.Close()); e != nil {
			return e
		}
		if e = verifyLayoutReadIdentity(src, f, before, n); e != nil {
			return e
		}
		seen[d.Digest] = d
		copied += count
		return check()
	}
	if e = copyBlob(md); e != nil {
		return report, e
	}
	if e = copyBlob(m.Config); e != nil {
		return report, e
	}
	for i, d := range m.Layers {
		if e = copyBlob(d); e != nil {
			return report, e
		}
		f, e := openPinnedStoreBlob(root, d)
		if e != nil {
			return report, e
		}
		tracked := &trackedLayerReader{input: f, hash: sha256.New()}
		decoded := sha256.New()
		var marker func(string) error
		if whiteouts {
			marker = func(n string) error { _, e := parseWhiteout(n); return e }
		}
		_, e = inspectLayerPolicy(ctx, tracked, d.MediaType, nil, decoded, marker)
		closeErr := f.Close()
		if e = errors.Join(e, closeErr); e != nil {
			return report, e
		}
		if tracked.bytes != d.Size || fmt.Sprintf("sha256:%x", tracked.hash.Sum(nil)) != d.Digest || fmt.Sprintf("sha256:%x", decoded.Sum(nil)) != ids[i] {
			return report, errors.New("export layer digest or DiffID mismatch")
		}
	}
	index, e := json.Marshal(struct {
		SchemaVersion int                  `json:"schemaVersion"`
		MediaType     string               `json:"mediaType"`
		Manifests     []ManifestDescriptor `json:"manifests"`
	}{2, MediaIndex, []ManifestDescriptor{md}})
	if e != nil {
		return report, e
	}
	if e = write("oci-layout", []byte("{\"imageLayoutVersion\":\"1.0.0\"}")); e != nil {
		return report, e
	}
	if e = exportSyncDir(root, "blobs/sha256"); e != nil {
		return report, e
	}
	if e = exportSyncDir(root, "blobs"); e != nil {
		return report, e
	}
	if e = write("index.json", index); e != nil {
		return report, e
	}
	if e = exportSyncDir(root, "."); e != nil {
		return report, e
	}
	if e = exportSyncDir(base, "."); e != nil {
		return report, e
	}
	verified, e := VerifyLayout(ctx, report.Directory)
	if e != nil || verified.ManifestCount != 1 || verified.VerifiedBlobs != len(seen) || verified.VerifiedBytes != copied {
		return report, errors.New("export layout verification failed")
	}
	reread, e := layoutBytes(root, "index.json", maxMetadataBytes)
	if e != nil || fmt.Sprintf("sha256:%x", sha256.Sum256(reread)) != fmt.Sprintf("sha256:%x", sha256.Sum256(index)) {
		return report, errors.New("export index changed")
	}
	if e = check(); e != nil {
		return report, e
	}
	report.IndexDigest = fmt.Sprintf("sha256:%x", sha256.Sum256(index))
	report.ManifestDigest = digest
	report.ManifestSize = size
	report.VerifiedBlobs = len(seen)
	report.VerifiedBytes = copied
	report.DiffIDsVerified = true
	report.ArchiveSubsetVerified = true
	return report, nil
}
