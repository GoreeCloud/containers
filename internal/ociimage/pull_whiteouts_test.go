package ociimage

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPinnedRegistryWhiteouts(t *testing.T) {
	for _, auth := range []bool{false, true} {
		t.Run(fmt.Sprint(auth), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			lower := makeLayer(t, layerEntry{name: "etc/old", size: 3, data: []byte("old")}, layerEntry{name: "keep", size: 1, data: []byte("k")})
			upper := makeLayer(t, layerEntry{name: "etc/new", size: 3, data: []byte("new")}, layerEntry{name: "etc/.wh..wh..opq"})
			var b bytes.Buffer
			z := gzip.NewWriter(&b)
			if _, e := z.Write(upper); e != nil {
				t.Fatal(e)
			}
			if e := z.Close(); e != nil {
				t.Fatal(e)
			}
			describe := func(b []byte, media string) ManifestDescriptor {
				return ManifestDescriptor{media, fmt.Sprintf("sha256:%x", sha256.Sum256(b)), int64(len(b))}
			}
			a, d := describe(lower, MediaLayer), describe(b.Bytes(), MediaLayerGzip)
			config, _ := json.Marshal(map[string]any{"os": "linux", "architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": []string{a.Digest, describe(upper, MediaLayer).Digest}}})
			c := describe(config, MediaConfig)
			manifest, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": MediaManifest, "config": c, "layers": []ManifestDescriptor{a, d}})
			m := describe(manifest, MediaManifest)
			const token = "synthetic-whiteout-token"
			var calls atomic.Int64
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				want := ""
				if auth {
					want = "Bearer " + token
				}
				if r.Header.Get("Authorization") != want {
					w.WriteHeader(401)
					return
				}
				switch r.URL.Path {
				case "/v2/example/test/manifests/" + m.Digest:
					w.Header().Set("Content-Type", MediaManifest)
					w.Write(manifest)
				case "/v2/example/test/blobs/" + c.Digest:
					w.Write(config)
				case "/v2/example/test/blobs/" + a.Digest:
					w.Write(lower)
				case "/v2/example/test/blobs/" + d.Digest:
					w.Write(b.Bytes())
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			client := srv.Client()
			if auth {
				var e error
				client, e = scopedClient(client.Transport, srv.URL, "example/test", m.Digest, token)
				if e != nil {
					t.Fatal(e)
				}
			}
			p := t.TempDir()
			os.Chmod(p, 0700)
			if _, e := s.pullAndStage(ctx, client, srv.URL, "example/test", m.Digest, m.Size, p); e == nil {
				t.Fatal("default accepted whiteout")
			}
			entries, e := os.ReadDir(p)
			if e != nil || len(entries) != 0 {
				t.Fatal("partial tree")
			}
			if calls.Load() != 4 {
				t.Fatal("unexpected request count")
			}
			r, e := s.pullAndStagePolicy(ctx, client, srv.URL, "example/test", m.Digest, m.Size, p, true)
			if e != nil || r.WhiteoutsApplied != 1 || r.ReusedBlobs != 4 || r.DownloadedBlobs != 0 || calls.Load() != 4 || !r.DiffIDsVerified || r.ExecutionAuthorized {
				t.Fatal(r, e)
			}
			if _, e = os.Stat(filepath.Join(r.Directory, "etc/old")); !os.IsNotExist(e) {
				t.Fatal("lower retained")
			}
			v, e := os.ReadFile(filepath.Join(r.Directory, "etc/new"))
			if e != nil || string(v) != "new" {
				t.Fatal("replacement missing")
			}
			raw, _ := json.Marshal(r)
			if strings.Contains(string(raw), token) {
				t.Fatal("token leaked")
			}
		})
	}
}
