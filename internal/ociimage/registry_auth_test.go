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

func TestScopedRegistryBearerEndToEnd(t *testing.T) {
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
	const synthetic = "synthetic-fixture-token"
	var calls atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+synthetic {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/v2/example/test/manifests/" + md.Digest:
			w.Header().Set("Content-Type", MediaManifest)
			w.Write(m)
		case "/v2/example/test/blobs/" + cd.Digest:
			w.Write(c)
		case "/v2/example/test/blobs/" + ld.Digest:
			w.Write(layer)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	client, e := scopedClient(srv.Client().Transport, srv.URL, "example/test", md.Digest, synthetic)
	if e != nil {
		t.Fatal(e)
	}
	parent := t.TempDir()
	os.Chmod(parent, 0700)
	report, e := s.pullAndStage(ctx, client, srv.URL, "example/test", md.Digest, md.Size, parent)
	if e != nil || report.DownloadedBlobs != 3 || !report.DiffIDsVerified || report.ExecutionAuthorized || calls.Load() != 3 {
		t.Fatal("synthetic auth pull failed")
	}
	b, _ := json.Marshal(report)
	if strings.Contains(string(b), synthetic) {
		t.Fatal("credential in report")
	}
	second, e := s.pullAndStage(ctx, client, srv.URL, "example/test", md.Digest, md.Size, parent)
	if e != nil || second.ReusedBlobs != 3 || calls.Load() != 3 {
		t.Fatal("cache reuse failed")
	}
	for _, u := range []string{srv.URL + "/v2/other/blobs/" + ld.Digest, srv.URL + "/v2/example/test/blobs/" + ld.Digest + "?x=1", srv.URL + "/v2/example/test/manifests/latest", "https://example.invalid/v2/example/test/blobs/" + ld.Digest} {
		req, _ := http.NewRequest("GET", u, nil)
		if _, e = client.Do(req); e == nil {
			t.Fatal("out-of-scope request accepted")
		}
	}
	if calls.Load() != 3 {
		t.Fatal("out-of-scope request reached registry")
	}
}
func TestRegistryBearerDoesNotFollowRedirect(t *testing.T) {
	var requests atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(200) }))
	defer target.Close()
	pin := "sha256:" + strings.Repeat("a", 64)
	src := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(302)
	}))
	defer src.Close()
	c, e := scopedClient(src.Client().Transport, src.URL, "repo", pin, "synthetic")
	if e != nil {
		t.Fatal(e)
	}
	resp, e := c.Get(src.URL + "/v2/repo/blobs/" + pin)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 302 || requests.Load() != 0 {
		t.Fatal("redirect followed")
	}
}
func TestRegistryBearerPrivateFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	p := t.TempDir()
	os.Chmod(p, 0700)
	f := filepath.Join(p, "credential")
	if e := os.WriteFile(f, []byte("synthetic\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if tok, e := readRegistryBearer(f); e != nil || tok != "synthetic" {
		t.Fatal("synthetic token rejected")
	}
	if e := os.Chmod(f, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readRegistryBearer(f); e == nil {
		t.Fatal("public token accepted")
	}
	if e := os.Chmod(f, 0600); e != nil {
		t.Fatal(e)
	}
	alias := filepath.Join(p, "alias")
	if e := os.Symlink(f, alias); e != nil {
		t.Fatal(e)
	}
	if _, e := readRegistryBearer(alias); e == nil {
		t.Fatal("symlink token accepted")
	}
	if e := os.WriteFile(f, []byte("synthetic\nextra"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readRegistryBearer(f); e == nil {
		t.Fatal("multiline token accepted")
	}
}
