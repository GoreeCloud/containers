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

func TestImportPinnedLayout(t *testing.T) {
	for _, mode := range []string{"valid", "whiteout", "wrong-index", "absent-manifest", "corrupt-layer", "bad-diff", "duplicate", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := testStore(t)
			layout := t.TempDir()
			blobs := filepath.Join(layout, "blobs/sha256")
			if e := os.MkdirAll(blobs, 0700); e != nil {
				t.Fatal(e)
			}
			write := func(name string, b []byte) {
				t.Helper()
				if e := os.WriteFile(name, b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			pin := func(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }
			put := func(media string, b []byte) ManifestDescriptor {
				d := ManifestDescriptor{media, pin(b), int64(len(b))}
				write(filepath.Join(blobs, d.Digest[7:]), b)
				return d
			}
			lower := makeLayer(t, layerEntry{name: "etc/old", size: 3, data: []byte("old")})
			upper := makeLayer(t, layerEntry{name: "etc/new", size: 3, data: []byte("new")})
			if mode == "whiteout" {
				upper = makeLayer(t, layerEntry{name: "etc/.wh.old"}, layerEntry{name: "etc/new", size: 3, data: []byte("new")})
			}
			var compressed bytes.Buffer
			z := gzip.NewWriter(&compressed)
			z.Write(upper)
			z.Close()
			l1, l2 := put(MediaLayer, lower), put(MediaLayerGzip, compressed.Bytes())
			ids := []string{pin(lower), pin(upper)}
			if mode == "bad-diff" {
				ids[1] = fmt.Sprintf("sha256:%064d", 0)
			}
			c, _ := json.Marshal(map[string]any{"os": "linux", "architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": ids}})
			cd := put(MediaConfig, c)
			m, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": MediaManifest, "config": cd, "layers": []ManifestDescriptor{l1, l2}})
			md := put(MediaManifest, m)
			children := []ManifestDescriptor{md}
			if mode == "duplicate" {
				children = append(children, md)
			}
			if mode == "absent-manifest" {
				children[0].Digest = fmt.Sprintf("sha256:%064d", 1)
			}
			index, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": MediaIndex, "manifests": children})
			write(filepath.Join(layout, "index.json"), index)
			write(filepath.Join(layout, "oci-layout"), []byte("{\"imageLayoutVersion\":\"1.0.0\"}"))
			ip := pin(index)
			if mode == "wrong-index" {
				ip = fmt.Sprintf("sha256:%064d", 0)
			}
			if mode == "corrupt-layer" {
				write(filepath.Join(blobs, l2.Digest[7:]), bytes.Repeat([]byte("x"), int(l2.Size)))
			}
			if mode == "symlink" {
				n := filepath.Join(blobs, l2.Digest[7:])
				os.Rename(n, n+".real")
				if e := os.Symlink(n+".real", n); e != nil {
					t.Fatal(e)
				}
			}
			parent := t.TempDir()
			os.Chmod(parent, 0700)
			r, e := s.ImportLayoutAndStage(ctx, layout, ip, md.Digest, md.Size, parent, mode == "whiteout")
			if mode != "valid" && mode != "whiteout" {
				if e == nil {
					t.Fatal("invalid layout accepted")
				}
				entries, _ := os.ReadDir(parent)
				if len(entries) != 0 {
					t.Fatal("partial tree")
				}
				return
			}
			if e != nil || r.ImportedBlobs != 4 || r.ReusedBlobs != 0 || !r.DiffIDsVerified || r.ExecutionAuthorized {
				t.Fatal(r, e)
			}
			b, e := os.ReadFile(filepath.Join(r.Directory, "etc/new"))
			if e != nil || string(b) != "new" {
				t.Fatal(string(b), e)
			}
			if mode == "whiteout" {
				if _, e = os.Stat(filepath.Join(r.Directory, "etc/old")); !os.IsNotExist(e) {
					t.Fatal("whiteout not applied")
				}
			}
			r, e = s.ImportLayoutAndStage(ctx, layout, ip, md.Digest, md.Size, parent, mode == "whiteout")
			if e != nil || r.ReusedBlobs != 4 || r.ImportedBlobs != 0 {
				t.Fatal(r, e)
			}
		})
	}
}
