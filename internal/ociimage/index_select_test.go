package ociimage

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPinnedIndexSelection(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	put := func(v any, media string) ManifestDescriptor {
		t.Helper()
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.Put(ctx, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		return ManifestDescriptor{media, r.Digest, r.Size}
	}
	pin := "sha256:" + strings.Repeat("a", 64)
	manifest := func(arch string) ManifestDescriptor {
		c := put(map[string]any{"os": "linux", "architecture": arch, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{pin}}}, MediaConfig)
		return put(map[string]any{"schemaVersion": 2, "mediaType": MediaManifest, "config": c, "layers": []ManifestDescriptor{{MediaLayer, pin, 1024}}}, MediaManifest)
	}
	amd, arm := manifest("amd64"), manifest("arm64")
	entry := func(d ManifestDescriptor, a string) map[string]any {
		return map[string]any{"mediaType": d.MediaType, "digest": d.Digest, "size": d.Size, "platform": map[string]string{"os": "linux", "architecture": a}}
	}
	idx := func(items ...map[string]any) ManifestDescriptor {
		return put(map[string]any{"schemaVersion": 2, "mediaType": MediaIndex, "manifests": items}, MediaIndex)
	}
	i := idx(entry(amd, "amd64"), entry(arm, "arm64"))
	for a, want := range map[string]ManifestDescriptor{"amd64": amd, "arm64": arm} {
		r, e := s.SelectIndex(ctx, i.Digest, i.Size, a)
		if e != nil || r.Manifest != want || !r.MetadataVerified || r.LayerContentsVerified || r.ExecutionAuthorized {
			t.Fatal(r, e)
		}
	}
	for _, bad := range []ManifestDescriptor{idx(entry(amd, "amd64"), entry(arm, "amd64")), idx(entry(arm, "amd64")), idx(entry(amd, "amd64"), entry(amd, "amd64")), idx(entry(amd, "arm64"))} {
		if _, e := s.SelectIndex(ctx, bad.Digest, bad.Size, "amd64"); e == nil {
			t.Fatal("unsafe index accepted")
		}
	}
	if _, e := s.SelectIndex(ctx, i.Digest, i.Size+1, "amd64"); e == nil {
		t.Fatal("wrong size accepted")
	}
	if _, e := s.SelectIndex(ctx, i.Digest, i.Size, "386"); e == nil {
		t.Fatal("unsupported arch accepted")
	}
}
