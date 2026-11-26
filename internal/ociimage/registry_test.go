package ociimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPinnedRegistryTLSAndNoBadCommit(t *testing.T) {
	s := testStore(t)
	data := []byte("first party synthetic blob")
	pin := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	for _, mode := range []string{"good", "wrong", "redirect", "auth", "encoding"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/example/test/blobs/"+pin {
					t.Error("unexpected request path")
				}
				switch mode {
				case "redirect":
					w.Header().Set("Location", "https://invalid.example/")
					w.WriteHeader(302)
					return
				case "auth":
					w.WriteHeader(401)
					return
				case "encoding":
					w.Header().Set("Content-Encoding", "gzip")
				}
				if mode == "wrong" {
					w.Write(bytes.Repeat([]byte("x"), len(data)))
					return
				}
				w.Write(data)
			}))
			defer srv.Close()
			c := srv.Client()
			c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
			r, e := s.fetchRegistryBlob(context.Background(), c, srv.URL, "example/test", pin, int64(len(data)))
			if mode == "good" {
				if e != nil || r.Digest != pin {
					t.Fatal(r, e)
				}
			} else if e == nil {
				t.Fatal("invalid registry response accepted")
			}
		})
	}
	files, e := os.ReadDir(filepath.Join(s.root, "blobs/sha256"))
	if e != nil || len(files) != 1 {
		t.Fatal("unverified blob published", e)
	}
}
func TestRegistryURLRejection(t *testing.T) {
	pin := fmt.Sprintf("sha256:%064x", 0)
	for _, origin := range []string{"http://registry.example", "https://user:secret@registry.example", "https://registry.example/path", "https://registry.example?x=1", "https://registry.example#x"} {
		if _, e := registryBlobURL(origin, "a/b", pin); e == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
	for _, repo := range []string{"../escape", "a//b", "a:latest", "a%2fb", "A/B"} {
		if _, e := registryBlobURL("https://registry.example", repo, pin); e == nil {
			t.Fatal("unsafe repo accepted")
		}
	}
}

func TestPinnedPutRejectsBeforePublication(t *testing.T) {
	s := testStore(t)
	expected := []byte("expected fixture bytes")
	pin := fmt.Sprintf("sha256:%x", sha256.Sum256(expected))
	for _, bad := range [][]byte{[]byte("wrong"), bytes.Repeat([]byte("x"), len(expected)), append(append([]byte{}, expected...), 1)} {
		if _, err := s.PutVerified(context.Background(), bytes.NewReader(bad), pin, int64(len(expected))); err == nil {
			t.Fatal("bad descriptor committed")
		}
		for _, name := range []string{"blobs/sha256", "tmp"} {
			entries, err := os.ReadDir(filepath.Join(s.root, name))
			if err != nil || len(entries) != 0 {
				t.Fatal("unexpected retained bytes", name, err)
			}
		}
	}
	info, err := s.PutVerified(context.Background(), bytes.NewReader(expected), pin, int64(len(expected)))
	if err != nil || info.Digest != pin {
		t.Fatal(info, err)
	}
}
