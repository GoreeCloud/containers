package ociimage

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhiteoutOrderedLowerOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	p := t.TempDir()
	os.Chmod(p, 0700)
	one := makeLayer(t, layerEntry{name: "etc/old", size: 1, data: []byte("x")}, layerEntry{name: "etc/nested/old", size: 1, data: []byte("y")}, layerEntry{name: "remove/child", size: 1, data: []byte("z")}, layerEntry{name: "keep", size: 1, data: []byte("k")})
	two := makeLayer(t, layerEntry{name: "etc/new", size: 1, data: []byte("n")}, layerEntry{name: "etc/.wh..wh..opq"}, layerEntry{name: ".wh.remove"}, layerEntry{name: "remove", size: 1, data: []byte("r")}, layerEntry{name: ".wh.missing"})
	r, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), layerInput(two)}, p)
	if e != nil || r.WhiteoutsApplied != 3 || r.ExecutionAuthorized {
		t.Fatal(r, e)
	}
	for n, w := range map[string]string{"etc/new": "n", "remove": "r", "keep": "k"} {
		b, e := os.ReadFile(filepath.Join(r.Directory, n))
		if e != nil || string(b) != w {
			t.Fatal(n, e)
		}
	}
	for _, n := range []string{"etc/old", "etc/nested", "etc/.wh..wh..opq", ".wh.remove", ".wh.missing"} {
		if _, e = os.Lstat(filepath.Join(r.Directory, n)); !os.IsNotExist(e) {
			t.Fatal("lower entry retained", n, e)
		}
	}
}
func TestWhiteoutRejectionAndRootOpaque(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	p := t.TempDir()
	os.Chmod(p, 0700)
	one := makeLayer(t, layerEntry{name: "old", size: 1, data: []byte("x")})
	for _, n := range []string{".wh.", ".wh..", ".wh...", "a/.wh.bad/sub", ".wh..wh.invalid"} {
		two := makeLayer(t, layerEntry{name: n})
		if _, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), layerInput(two)}, p); e == nil {
			t.Fatal("invalid marker", n)
		}
	}
	bad := makeLayer(t, layerEntry{name: ".wh.old", kind: tar.TypeDir})
	if _, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), layerInput(bad)}, p); e == nil {
		t.Fatal("directory marker accepted")
	}
	two := makeLayer(t, layerEntry{name: ".wh.old", size: 1, data: []byte("x")})
	if _, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), layerInput(two)}, p); e == nil {
		t.Fatal("payload marker accepted")
	}
	good := makeLayer(t, layerEntry{name: "new"}, layerEntry{name: ".wh..wh..opq"})
	wrong := layerInput(good)
	wrong.DiffID = "sha256:" + strings.Repeat("0", 64)
	if _, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), wrong}, p); e == nil {
		t.Fatal("bad diff ID accepted")
	}
	entries, e := os.ReadDir(p)
	if e != nil || len(entries) != 0 {
		t.Fatal("partial staging retained", e)
	}
	r, e := StageLayersWithWhiteouts(context.Background(), []LayerInput{layerInput(one), layerInput(good)}, p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(r.Directory, "old")); !os.IsNotExist(e) {
		t.Fatal("root opaque retained old")
	}
	if _, e = os.Stat(filepath.Join(r.Directory, "new")); e != nil {
		t.Fatal(e)
	}
}
