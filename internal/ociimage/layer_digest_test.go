package ociimage

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestVerifyLayerDigestMatchesExactRawAndGzipStream(t *testing.T) {
	raw := makeLayer(t, layerEntry{name: "etc/config", size: 3, data: []byte("abc")})
	for _, media := range []string{MediaLayer, MediaLayerGzip} {
		source := raw
		if media == MediaLayerGzip {
			var buffer bytes.Buffer
			zw := gzip.NewWriter(&buffer)
			if _, err := zw.Write(raw); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			source = buffer.Bytes()
		}
		sum := sha256.Sum256(source)
		digest := fmt.Sprintf("sha256:%x", sum)
		r, err := VerifyLayerWithDigest(context.Background(), bytes.NewReader(source), media, digest)
		if err != nil {
			t.Fatal(err)
		}
		if !r.DigestMatches || !r.ArchiveSubsetValid || r.SourceProvenanceVerified ||
			r.ExtractionAuthorized || r.Layer.FileCount != 1 || r.CompressedBytes != int64(len(source)) {
			t.Fatalf("verification misrepresented provenance or bytes %+v", r)
		}
		stale := "sha256:" + fmt.Sprintf("%064x", 0)
		if _, err := VerifyLayerWithDigest(context.Background(), bytes.NewReader(source), media, stale); err == nil {
			t.Fatal("incorrect expected digest accepted")
		}
	}
}
func TestVerifyLayerRejectsCorrectDigestOfHostileArchive(t *testing.T) {
	source := makeLayer(t, layerEntry{name: "../path-escape"})
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(source))
	if _, err := VerifyLayerWithDigest(context.Background(), bytes.NewReader(source), MediaLayer, digest); err == nil {
		t.Fatal("hash match bypassed hostile tar structure")
	}
	if _, err := VerifyLayerWithDigest(context.Background(), bytes.NewReader(source), MediaLayer, "sha256:not-a-digest"); err == nil {
		t.Fatal("malformed digest accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyLayerWithDigest(ctx, bytes.NewReader(source), MediaLayer, digest); err == nil {
		t.Fatal("cancelled layer verification accepted")
	}
}
