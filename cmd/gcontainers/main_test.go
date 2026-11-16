package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOnlyCommands(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"version"}, &b); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != version {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := run([]string{"doctor"}, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"production_ready": false`) {
		t.Fatal(b.String())
	}
}
func TestInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"validate"}, {"image"}, {"image", "verify", "missing"}, {"validate", "not-a-bundle"}, {"unknown"},
		{"oci", "unknown"}, {"oci", "create", "--id", "../bad", "--experimental"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestImageImportIsExplicitAndRootless(t *testing.T) {
	var out bytes.Buffer
	source := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(source, []byte("synthetic bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "image-store")
	if err := run([]string{"image", "import", "--store", destination, "--file", source}, &out); err == nil {
		t.Fatal("image import without experimental consent accepted")
	}
	if os.Geteuid() == 0 {
		t.Skip("image import intentionally forbids root")
	}
	out.Reset()
	if err := run([]string{"image", "import", "--store", destination, "--file", source, "--experimental"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sha256:") {
		t.Fatalf("digest missing: %s", out.String())
	}
	if strings.Contains(out.String(), "synthetic bytes") {
		t.Fatal("source bytes leaked into CLI output")
	}
}

func TestLayerInspectionCLI(t *testing.T) {
	var bytesOut bytes.Buffer
	var tarBytes bytes.Buffer
	writer := tar.NewWriter(&tarBytes)
	if err := writer.WriteHeader(&tar.Header{Name: "readme.txt", Mode: 0644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(filename, tarBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"image", "inspect-layer", filename, "tar"}, &bytesOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bytesOut.String(), `"fileCount": 1`) || !strings.Contains(bytesOut.String(), `"unpackedBytes": 2`) {
		t.Fatalf("invalid read-only output: %s", bytesOut.String())
	}
	if err := run([]string{"image", "inspect-layer", filename, "tar+zstd"}, &bytesOut); err == nil {
		t.Fatal("unsupported compression accepted")
	}
}

func TestAuditCLINeverCreatesAnImageStore(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-store")
	var out bytes.Buffer
	if err := run([]string{"image", "audit", "--store", missing}, &out); err == nil {
		t.Fatal("missing image store accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only audit created a directory")
	}
	if err := run([]string{"image", "audit", "--store"}, &out); err == nil {
		t.Fatal("missing path accepted")
	}
}
