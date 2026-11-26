package volume

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyBackupWithoutDestination(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	ctx := context.Background()
	p, b := t.TempDir(), t.TempDir()
	os.Chmod(p, 0700)
	os.Chmod(b, 0700)
	if _, e := Create(ctx, p, "source"); e != nil {
		t.Fatal(e)
	}
	f := filepath.Join(p, "source", "data")
	if e := os.WriteFile(f, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Backup(ctx, p, "source", b)
	if e != nil {
		t.Fatal(e)
	}
	v, e := VerifyBackup(ctx, r.Directory, r.ContentSHA256)
	if e != nil || !v.DigestMatches || !v.ReadOnly || v.Files != 1 || v.Bytes != 8 || v.ApplicationConsistent {
		t.Fatal(v, e)
	}
	if _, e = VerifyBackup(ctx, r.Directory, "sha256:"+strings.Repeat("0", 64)); e == nil {
		t.Fatal("wrong pin accepted")
	}
	if e = os.WriteFile(filepath.Join(r.Directory, "data"), []byte("modified"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = VerifyBackup(ctx, r.Directory, r.ContentSHA256); e == nil {
		t.Fatal("corrupt backup accepted")
	}
	entries, e := os.ReadDir(b)
	if e != nil || len(entries) != 1 {
		t.Fatal("verification mutated inventory", e)
	}
	original, e := os.ReadFile(f)
	if e != nil || string(original) != "original" {
		t.Fatal("source changed")
	}
}
