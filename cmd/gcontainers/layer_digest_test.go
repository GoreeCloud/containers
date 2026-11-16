package main

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageVerifyLayerCLIHonorsPinnedDescriptorAndNoExtraction(t *testing.T) {
	dir := t.TempDir()
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	content := []byte("synthetic-layer-content")
	if err := writer.WriteHeader(&tar.Header{Name: "etc/preview", Mode: 0600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "layer.tar")
	if err := os.WriteFile(source, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data.Bytes()))
	var out bytes.Buffer
	if err := run([]string{"image", "verify-layer", source, "tar", digest}, &out); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{
		`"digestMatches": true`,
		`"archiveSubsetValid": true`,
		`"sourceProvenanceVerified": false`,
		`"extractionAuthorized": false`,
	} {
		if !strings.Contains(out.String(), item) {
			t.Fatalf("missing conservative status %q", item)
		}
	}
	out.Reset()
	if err := run([]string{"image", "verify-layer", source, "tar", "sha256:" + strings.Repeat("0", 64)}, &out); err == nil {
		t.Fatal("wrong content descriptor accepted")
	}
	alias := filepath.Join(dir, "link.tar")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"image", "verify-layer", alias, "tar", digest}, &out); err == nil {
		t.Fatal("symlink layer accepted")
	}
}
