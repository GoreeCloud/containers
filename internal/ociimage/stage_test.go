package ociimage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStageVerifiedLayer(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot test")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	src := makeLayer(t, layerEntry{name: "a/b", size: 3, data: []byte("abc")}, layerEntry{name: "a/", kind: tar.TypeDir})
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(src))
	r, e := StageLayer(context.Background(), bytes.NewReader(src), MediaLayer, digest, int64(len(src)), parent)
	if e != nil {
		t.Fatal(e)
	}
	if !r.DigestMatches || !r.DescriptorSizeVerified || r.ExecutionAuthorized || r.SourceProvenanceVerified {
		t.Fatalf("bad report: %+v", r)
	}
	b, e := os.ReadFile(filepath.Join(r.Directory, "a/b"))
	if e != nil || string(b) != "abc" {
		t.Fatal("bad staged content", e)
	}
	fi, e := os.Stat(filepath.Join(r.Directory, "a/b"))
	if e != nil || fi.Mode().Perm() != 0600 {
		t.Fatal("unsafe mode", e)
	}
}
func TestStageLayerFailureLeavesNoTree(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot test")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	good := makeLayer(t, layerEntry{name: "x", size: 1, data: []byte("x")})
	bad := makeLayer(t, layerEntry{name: "x", size: 1, data: []byte("x")}, layerEntry{name: "escape", kind: tar.TypeSymlink, link: "/etc"})
	for _, src := range [][]byte{good, bad, good[:len(good)-512]} {
		if _, e := StageLayer(context.Background(), bytes.NewReader(src), MediaLayer, fmt.Sprintf("sha256:%064x", 0), int64(len(src)), parent); e == nil {
			t.Fatal("invalid stage accepted")
		}
		entries, e := os.ReadDir(parent)
		if e != nil || len(entries) != 0 {
			t.Fatal("failed stage left data", e)
		}
	}
}

func TestStageGzipAndExactSize(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot test")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	raw := makeLayer(t, layerEntry{name: "x", size: 1, data: []byte("x")})
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, e := gz.Write(raw); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	src := compressed.Bytes()
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(src))
	if _, e := StageLayer(context.Background(), bytes.NewReader(src), MediaLayerGzip, digest, int64(len(src))+1, parent); e == nil {
		t.Fatal("incorrect descriptor size accepted")
	}
	if entries, e := os.ReadDir(parent); e != nil || len(entries) != 0 {
		t.Fatal("failed staging not cleaned", e)
	}
	if _, e := StageLayer(context.Background(), bytes.NewReader(src), MediaLayerGzip, digest, int64(len(src)), parent); e != nil {
		t.Fatal(e)
	}
}
