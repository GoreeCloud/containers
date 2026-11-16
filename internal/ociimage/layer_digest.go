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
	ExpectedDigest          string `json:"expectedDigest"`
	CompressedBytes         int64  `json:"compressedBytes"`
	ExpectedDescriptorBytes int64  `json:"expectedDescriptorBytes,omitempty"`
	DescriptorSizeVerified  bool   `json:"descriptorSizeVerified"`

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

// VerifyLayerWithDescriptor checks exact source bytes and the expected size.
// The caller-supplied descriptor itself is not authenticated.
func VerifyLayerWithDescriptor(ctx context.Context, input io.Reader, media, digest string, expectedBytes int64) (VerifiedLayerReport, error) {
	var empty VerifiedLayerReport
	if expectedBytes <= 0 || expectedBytes > MaxLayerCompressedBytes {
		return empty, errors.New("OCI descriptor compressed byte size out of bounds")
	}
	report, err := VerifyLayerWithDigest(ctx, input, media, digest)
	if err != nil {
		return empty, err
	}
	if report.CompressedBytes != expectedBytes {
		return empty, errors.New("OCI layer compressed byte size differs from descriptor")
	}
	report.ExpectedDescriptorBytes = expectedBytes
	report.DescriptorSizeVerified = true
	return report, nil
}
