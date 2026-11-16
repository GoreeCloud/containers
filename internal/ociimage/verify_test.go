package ociimage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	root   string
	config string
	layer  string
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func writeBlob(t *testing.T, root string, bytes []byte) (string, string) {
	t.Helper()
	hash := sha256.Sum256(bytes)
	encoded := hex.EncodeToString(hash[:])
	file := filepath.Join(root, "blobs", "sha256", encoded)
	if err := os.WriteFile(file, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	return "sha256:" + encoded, file
}
func newFixture(t *testing.T) fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "blobs", "sha256"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "oci-layout"), encode(t, map[string]string{"imageLayoutVersion": "1.0.0"}), 0600); err != nil {
		t.Fatal(err)
	}
	configBytes := []byte("{}")
	configDigest, configPath := writeBlob(t, root, configBytes)
	layerBytes := []byte("synthetic-layer-bytes-no-unpack")
	layerDigest, layerPath := writeBlob(t, root, layerBytes)
	manifest := map[string]any{
		"schemaVersion": 2, "mediaType": MediaManifest,
		"config": map[string]any{"mediaType": MediaConfig, "digest": configDigest, "size": len(configBytes)},
		"layers": []any{map[string]any{"mediaType": MediaLayer, "digest": layerDigest, "size": len(layerBytes)}},
	}
	manifestBytes := encode(t, manifest)
	manifestDigest, _ := writeBlob(t, root, manifestBytes)
	index := map[string]any{"schemaVersion": 2, "mediaType": MediaIndex,
		"manifests": []any{map[string]any{"mediaType": MediaManifest, "digest": manifestDigest, "size": len(manifestBytes)}}}
	if err := os.WriteFile(filepath.Join(root, "index.json"), encode(t, index), 0600); err != nil {
		t.Fatal(err)
	}
	return fixture{root: root, config: configPath, layer: layerPath}
}
func TestVerifyValidSyntheticLayout(t *testing.T) {
	f := newFixture(t)
	r, err := VerifyLayout(context.Background(), f.root)
	if err != nil {
		t.Fatal(err)
	}
	if r.ManifestCount != 1 || r.VerifiedBlobs != 3 || r.VerifiedBytes <= 0 {
		t.Fatalf("bad report: %+v", r)
	}
}
func TestTamperedBlobRejected(t *testing.T) {
	f := newFixture(t)
	original, err := os.ReadFile(f.layer)
	if err != nil {
		t.Fatal(err)
	}
	original[0] ^= 0xff
	if err := os.WriteFile(f.layer, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("tampered layer accepted")
	}
}
func TestSymlinkBlobRejected(t *testing.T) {
	f := newFixture(t)
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", f.config); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("symlinked blob accepted")
	}
}
func TestDuplicateKeyAndTrailingDataRejected(t *testing.T) {
	f := newFixture(t)
	indexPath := filepath.Join(f.root, "index.json")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), "\"schemaVersion\":2", "\"schemaVersion\":2,\"schemaVersion\":2", 1))
	if err := os.WriteFile(indexPath, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("duplicate OCI key accepted")
	}
	if err := os.WriteFile(indexPath, []byte("{}{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
func TestInvalidIndexAndURLRejected(t *testing.T) {
	f := newFixture(t)
	indexPath := filepath.Join(f.root, "index.json")
	cases := []map[string]any{
		{"schemaVersion": 3, "manifests": []any{}},
		{"schemaVersion": 2, "manifests": []any{}},
		{"schemaVersion": 2, "manifests": []any{map[string]any{
			"mediaType": MediaManifest, "digest": "sha256:../../etc/passwd", "size": 9}}},
		{"schemaVersion": 2, "manifests": []any{map[string]any{
			"mediaType": MediaManifest, "digest": "sha256:" + strings.Repeat("a", 64), "size": 5, "urls": []string{"https://evil.example/x"}}}},
		{"schemaVersion": 2, "manifests": []any{map[string]any{
			"mediaType": "unknown", "digest": "sha256:" + strings.Repeat("a", 64), "size": 5}}},
	}
	for _, item := range cases {
		if err := os.WriteFile(indexPath, encode(t, item), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := VerifyLayout(context.Background(), f.root); err == nil {
			t.Fatalf("invalid index accepted: %v", item)
		}
	}
}
func TestIndexTooLarge(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.root, "index.json"), []byte(strings.Repeat(" ", int(maxMetadataBytes)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("oversize index accepted")
	}
}
func TestCancelledVerification(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := VerifyLayout(ctx, f.root); err == nil {
		t.Fatal("cancelled verification accepted")
	}
}

func TestConflictingDescriptorRejected(t *testing.T) {
	f := newFixture(t)
	indexFile := filepath.Join(f.root, "index.json")
	original, err := os.ReadFile(indexFile)
	if err != nil {
		t.Fatal(err)
	}
	var idx map[string]any
	if err := json.Unmarshal(original, &idx); err != nil {
		t.Fatal(err)
	}
	entries := idx["manifests"].([]any)
	first := entries[0].(map[string]any)
	conflicting := map[string]any{"mediaType": first["mediaType"], "digest": first["digest"], "size": first["size"].(float64) + 1}
	idx["manifests"] = append(entries, conflicting)
	if err := os.WriteFile(indexFile, encode(t, idx), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("conflicting OCI descriptor accepted")
	}
}

func FuzzOCIImageMetadataJSON(f *testing.F) {
	f.Add([]byte("{\"schemaVersion\":2,\"manifests\":[]}"))
	f.Add([]byte("{\"schemaVersion\":2,\"schemaVersion\":1}"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip()
		}
		_ = uniqueJSON(b)
	})
}

func TestLayoutVerifierRejectsSymlinkedPaths(t *testing.T) {
	f := newFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(f.root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), alias); err == nil {
		t.Fatal("symlinked layout root accepted")
	}
	index := filepath.Join(f.root, "index.json")
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", index); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("symlinked OCI index accepted")
	}
}
func TestLayoutVerifierRejectsReplacedBlobDirectory(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "blobs", "sha256")
	if err := os.Rename(dir, dir+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("symlinked OCI blob directory accepted")
	}
}
func TestLayoutVerifierRejectsTamperedManifest(t *testing.T) {
	f := newFixture(t)
	raw, err := os.ReadFile(filepath.Join(f.root, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	entry := index["manifests"].([]any)[0].(map[string]any)
	digest := entry["digest"].(string)
	path := filepath.Join(f.root, "blobs", "sha256", strings.TrimPrefix(digest, "sha256:"))
	manifest, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) == 0 {
		t.Fatal("empty fixture manifest")
	}
	manifest[0] ^= 1
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyLayout(context.Background(), f.root); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}
