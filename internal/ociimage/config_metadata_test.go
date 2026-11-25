package ociimage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutConfigMetadataAdmission(t *testing.T) {
	for _, tc := range []struct {
		name     string
		config   []byte
		layers   any
		sizeNull bool
		valid    bool
	}{
		{"valid", []byte("{}"), []any{}, false, true},
		{"malformed", []byte("{"), []any{}, false, false},
		{"not-object", []byte("[]"), []any{}, false, false},
		{"null-config", []byte("null"), []any{}, false, false},
		{"duplicate", []byte(`{"architecture":"amd64","ARCHITECTURE":"arm64"}`), []any{}, false, false},
		{"null-layers", []byte("{}"), nil, false, false},
		{"null-size", []byte{}, []any{}, true, false},
		{"too-large", make([]byte, maxMetadataBytes+1), []any{}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			digest, _ := writeBlob(t, f.root, tc.config)
			var size any = len(tc.config)
			if tc.sizeNull {
				size = nil
			}
			m := encode(t, map[string]any{"schemaVersion": 2, "config": map[string]any{"mediaType": MediaConfig, "digest": digest, "size": size}, "layers": tc.layers})
			md, _ := writeBlob(t, f.root, m)
			idx := encode(t, map[string]any{"schemaVersion": 2, "manifests": []any{map[string]any{"mediaType": MediaManifest, "digest": md, "size": len(m)}}})
			if err := os.WriteFile(filepath.Join(f.root, "index.json"), idx, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := VerifyLayout(context.Background(), f.root)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t: %v", tc.valid, err)
			}
		})
	}
}
func TestDescriptorRejectsMissingSize(t *testing.T) {
	d := map[string]json.RawMessage{"mediaType": json.RawMessage(`"` + MediaConfig + `"`), "digest": json.RawMessage(`"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)}
	if _, err := parseDescriptor(d); err == nil {
		t.Fatal("missing size accepted")
	}
}
