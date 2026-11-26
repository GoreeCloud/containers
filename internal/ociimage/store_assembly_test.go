package ociimage

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreManifestAssembly(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	first := makeLayer(t, layerEntry{name: "etc/a", size: 3, data: []byte("old")})
	second := makeLayer(t, layerEntry{name: "etc/a", size: 3, data: []byte("new")}, layerEntry{name: "etc/b", size: 1, data: []byte("b")})
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, e := gz.Write(second); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	put := func(b []byte, media string) ManifestDescriptor {
		t.Helper()
		r, e := s.Put(ctx, bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		return ManifestDescriptor{media, r.Digest, r.Size}
	}
	layers := []ManifestDescriptor{put(first, MediaLayer), put(compressed.Bytes(), MediaLayerGzip)}
	for _, mode := range []string{"good", "bad-diff", "bad-config", "wrong-size", "missing-layer", "corrupt-layer"} {
		t.Run(mode, func(t *testing.T) {
			ids := []string{fmt.Sprintf("sha256:%x", sha256.Sum256(first)), fmt.Sprintf("sha256:%x", sha256.Sum256(second))}
			if mode == "bad-diff" {
				ids[1] = fmt.Sprintf("sha256:%064x", 0)
			}
			osName := "linux"
			if mode == "bad-config" {
				osName = "windows"
			}
			c, _ := json.Marshal(map[string]any{"os": osName, "architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": ids}, "config": map[string]any{"Env": []string{"PRIVATE=synthetic"}}})
			cd := put(c, MediaConfig)
			ls := append([]ManifestDescriptor(nil), layers...)
			if mode == "wrong-size" {
				ls[1].Size++
			}
			if mode == "missing-layer" {
				ls[1].Digest = fmt.Sprintf("sha256:%064x", 3)
			}
			if mode == "corrupt-layer" {
				p := filepath.Join(s.root, "blobs/sha256", ls[1].Digest[7:])
				if e := os.WriteFile(p, bytes.Repeat([]byte("x"), int(ls[1].Size)), 0600); e != nil {
					t.Fatal(e)
				}
			}
			m, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": MediaManifest, "config": cd, "layers": ls})
			// Store the manifest before corrupting an already committed layer, because
			// Put may audit existing bytes. For the final corruption case use the
			// previously admitted matching manifest.
			md := ManifestDescriptor{MediaManifest, fmt.Sprintf("sha256:%x", sha256.Sum256(m)), int64(len(m))}
			if mode != "corrupt-layer" {
				md = put(m, MediaManifest)
			}
			parent := t.TempDir()
			if e := os.Chmod(parent, 0700); e != nil {
				t.Fatal(e)
			}
			r, e := s.AssembleManifest(ctx, md.Digest, md.Size, parent)
			if mode == "good" {
				if e != nil || !r.DiffIDsVerified || r.ExecutionAuthorized || r.SourceProvenanceVerified || r.LayerCount != 2 {
					t.Fatal(r, e)
				}
				b, e := os.ReadFile(filepath.Join(r.Directory, "etc/a"))
				if e != nil || string(b) != "new" {
					t.Fatal(string(b), e)
				}
			} else {
				if e == nil {
					t.Fatal("invalid assembly accepted")
				}
				entries, e := os.ReadDir(parent)
				if e != nil || len(entries) != 0 {
					t.Fatal("partial tree retained", e)
				}
			}
		})
	}
}
