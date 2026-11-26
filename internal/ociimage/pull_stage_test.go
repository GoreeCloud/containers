package ociimage

import (
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

func TestPinnedRegistryPullStageReuseAndFailure(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			layer := makeLayer(t, layerEntry{name: "data", size: 7, data: []byte("fixture")})
			describe := func(b []byte, media string) ManifestDescriptor {
				return ManifestDescriptor{media, fmt.Sprintf("sha256:%x", sha256.Sum256(b)), int64(len(b))}
			}
			ld := describe(layer, MediaLayer)
			c, _ := json.Marshal(map[string]any{"os": "linux", "architecture": "amd64", "rootfs": map[string]any{"type": "layers", "diff_ids": []string{ld.Digest}}})
			cd := describe(c, MediaConfig)
			m, _ := json.Marshal(map[string]any{"schemaVersion": 2, "mediaType": MediaManifest, "config": cd, "layers": []ManifestDescriptor{ld}})
			md := describe(m, MediaManifest)
			var calls atomic.Int64
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("Authorization") != "" || r.Method != "GET" {
					t.Error("unexpected request")
				}
				switch r.URL.Path {
				case "/v2/example/test/manifests/" + md.Digest:
					w.Header().Set("Content-Type", MediaManifest)
					w.Write(m)
				case "/v2/example/test/blobs/" + cd.Digest:
					w.Write(c)
				case "/v2/example/test/blobs/" + ld.Digest:
					if bad {
						w.Write([]byte(strings.Repeat("x", len(layer))))
					} else {
						w.Write(layer)
					}
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			client := srv.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			parent := t.TempDir()
			if e := os.Chmod(parent, 0700); e != nil {
				t.Fatal(e)
			}
			report, e := s.pullAndStage(ctx, client, srv.URL, "example/test", md.Digest, md.Size, parent)
			if bad {
				if e == nil {
					t.Fatal("bad layer admitted")
				}
				entries, e := os.ReadDir(parent)
				if e != nil || len(entries) != 0 {
					t.Fatal("failed pull left staging", e)
				}
				if _, e = s.Verify(ctx, ld.Digest); e == nil {
					t.Fatal("invalid blob published")
				}
				return
			}
			if e != nil || report.DownloadedBlobs != 3 || !report.DiffIDsVerified || report.ExecutionAuthorized || calls.Load() != 3 {
				t.Fatal(report, e, calls.Load())
			}
			data, e := os.ReadFile(filepath.Join(report.Directory, "data"))
			if e != nil || string(data) != "fixture" {
				t.Fatal(string(data), e)
			}
			second, e := s.pullAndStage(ctx, client, srv.URL, "example/test", md.Digest, md.Size, parent)
			if e != nil || second.ReusedBlobs != 3 || second.DownloadedBlobs != 0 || calls.Load() != 3 || second.Directory == report.Directory {
				t.Fatal(second, e, calls.Load())
			}
		})
	}
}
