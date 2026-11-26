package ociimage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestManifestDiscoveryPinnedDescriptors(t *testing.T) {
	c := fmt.Sprintf("sha256:%064x", 1)
	l := fmt.Sprintf("sha256:%064x", 2)
	base := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":24},"layers":[{"mediaType":%q,"digest":%q,"size":1024}],"annotations":{"private":"synthetic"}}`, MediaManifest, MediaConfig, c, MediaLayer, l)
	for _, mode := range []string{"good", "bad-digest", "redirect", "wrong-media", "duplicate", "external-url", "conflict"} {
		t.Run(mode, func(t *testing.T) {
			raw := base
			if mode == "duplicate" {
				raw = strings.Replace(raw, `"schemaVersion":2`, `"schemaVersion":2,"SchemaVersion":2`, 1)
			}
			if mode == "external-url" {
				raw = strings.Replace(raw, `"size":1024`, `"size":1024,"urls":["https://invalid.example"]`, 1)
			}
			if mode == "conflict" {
				raw = strings.Replace(raw, l, c, 1)
			}
			pin := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw)))
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/example/test/manifests/"+pin || r.Header.Get("Accept") != MediaManifest {
					t.Error("incorrect pinned request")
				}
				if mode == "redirect" {
					w.Header().Set("Location", "https://invalid.example/")
					w.WriteHeader(302)
					return
				}
				media := MediaManifest
				if mode == "wrong-media" {
					media = MediaIndex
				}
				w.Header().Set("Content-Type", media)
				body := raw
				if mode == "bad-digest" {
					body = strings.Replace(raw, "synthetic", "different", 1)
				}
				fmt.Fprint(w, body)
			}))
			defer srv.Close()
			client := srv.Client()
			client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			r, e := discoverManifest(context.Background(), client, srv.URL, "example/test", pin, int64(len(raw)))
			if mode == "good" {
				if e != nil || !r.DigestMatches || r.SourceProvenanceVerified || r.ExecutionAuthorized || len(r.Layers) != 1 || r.Layers[0].Digest != l {
					t.Fatal(r, e)
				}
			} else if e == nil {
				t.Fatal("invalid manifest accepted", mode)
			}
		})
	}
}
