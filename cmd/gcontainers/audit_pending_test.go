package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoreeCloud/containers/internal/ociimage"
)

func TestAuditCLIReturnsNonzeroForPendingImport(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless required")
	}
	dir := filepath.Join(t.TempDir(), "store")
	if _, err := ociimage.OpenStore(dir); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(dir, "tmp", ".ingest-"+strings.Repeat("a", 32))
	if err := os.WriteFile(pending, []byte("opaque partial content"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"image", "audit", "--store", dir}, &out); err == nil {
		t.Fatal("pending import incorrectly returned healthy CLI exit status")
	}
	if !strings.Contains(out.String(), "\"operatorReviewRequired\": true") ||
		!strings.Contains(out.String(), "\"verified\": false") {
		t.Fatalf("unsafe staging report: %s", out.String())
	}
	if _, err := os.Stat(pending); err != nil {
		t.Fatal("audit deleted a pending import")
	}
}
