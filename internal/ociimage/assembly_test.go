package ociimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func layerInput(data []byte) LayerInput {
	return LayerInput{Reader: bytes.NewReader(data), Media: MediaLayer, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), Size: int64(len(data))}
}
func TestOrderedLayerAssembly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	one := makeLayer(t, layerEntry{name: "etc/a", size: 3, data: []byte("old")})
	two := makeLayer(t, layerEntry{name: "etc/a", size: 3, data: []byte("new")}, layerEntry{name: "etc/b", size: 1, data: []byte("b")})
	r, e := StageLayers(context.Background(), []LayerInput{layerInput(one), layerInput(two)}, parent)
	if e != nil {
		t.Fatal(e)
	}
	for n, want := range map[string]string{"etc/a": "new", "etc/b": "b"} {
		b, e := os.ReadFile(filepath.Join(r.Directory, n))
		if e != nil || string(b) != want {
			t.Fatal(n, e)
		}
	}
	if r.ExecutionAuthorized || r.SourceProvenanceVerified {
		t.Fatal("unsafe admission")
	}
}
func TestAssemblyRejectsWholeTree(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	parent := t.TempDir()
	if e := os.Chmod(parent, 0700); e != nil {
		t.Fatal(e)
	}
	one := makeLayer(t, layerEntry{name: "x", size: 1, data: []byte("a")})
	two := makeLayer(t, layerEntry{name: "x/y", size: 1, data: []byte("b")})
	if _, e := StageLayers(context.Background(), []LayerInput{layerInput(one), layerInput(two)}, parent); e == nil {
		t.Fatal("type collision accepted")
	}
	bad := layerInput(one)
	bad.Digest = fmt.Sprintf("sha256:%064x", 0)
	if _, e := StageLayers(context.Background(), []LayerInput{layerInput(one), bad}, parent); e == nil {
		t.Fatal("bad digest accepted")
	}
	entries, e := os.ReadDir(parent)
	if e != nil || len(entries) != 0 {
		t.Fatal("partial tree exposed", e)
	}
}
