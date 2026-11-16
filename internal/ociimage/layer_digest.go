package ociimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
)

// VerifiedLayerReport is a source-anchored *read-only* archive preflight.
// A matching caller-supplied digest is NOT a trusted signature, rootfs,
// secure unpack, source provenance claim or authorization to run.
type VerifiedLayerReport struct {
	ExpectedDigest           string      `json:"expectedDigest"`
	CompressedBytes          int64       `json:"compressedBytes"`
	DigestMatches            bool        `json:"digestMatches"`
	ArchiveSubsetValid       bool        `json:"archiveSubsetValid"`
	SourceProvenanceVerified bool        `json:"sourceProvenanceVerified"`
	ExtractionAuthorized     bool        `json:"extractionAuthorized"`
	Layer                    LayerReport `json:"layer"`
}
type trackedLayerReader struct {
	input io.Reader
	hash  hash.Hash
	bytes int64
}

func (r *trackedLayerReader) Read(p []byte) (int, error) {
	n, err := r.input.Read(p)
	if n < 0 || n > len(p) {
		return 0, errors.New("invalid OCI layer reader count")
	}
	if n > 0 {
		r.bytes += int64(n)
		_, _ = r.hash.Write(p[:n])
	}
	return n, err
}

// VerifyLayerWithDigest hashes the exact compressed/raw bytes consumed while
// InspectLayer validates its strict, no-whiteout/no-link preview subset.
// Hashing happens in the SAME pass to avoid separate-open file races.
func VerifyLayerWithDigest(ctx context.Context, input io.Reader, media, digest string) (VerifiedLayerReport, error) {
	empty := VerifiedLayerReport{}
	if ctx == nil || input == nil {
		return empty, errors.New("context and OCI layer input required")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if !digestPattern.MatchString(digest) {
		return empty, errors.New("expected SHA256 must be exact lowercase digest")
	}
	checked := &trackedLayerReader{input: input, hash: sha256.New()}
	inspected, err := InspectLayer(ctx, checked, media)
	if err != nil {
		return empty, err
	}
	if checked.bytes == 0 || checked.bytes > MaxLayerCompressedBytes {
		return empty, errors.New("OCI layer compressed byte limit invalid")
	}
	if "sha256:"+hex.EncodeToString(checked.hash.Sum(nil)) != digest {
		return empty, errors.New("OCI layer does not match expected descriptor digest")
	}
	return VerifiedLayerReport{
		ExpectedDigest: digest, CompressedBytes: checked.bytes,
		DigestMatches: true, ArchiveSubsetValid: true, SourceProvenanceVerified: false,
		ExtractionAuthorized: false, Layer: inspected,
	}, nil
}
