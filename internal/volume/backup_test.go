package volume

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot")
	}
	ctx := context.Background()
	store := t.TempDir()
	backups := t.TempDir()
	os.Chmod(store, 0700)
	os.Chmod(backups, 0700)
	if _, e := Create(ctx, store, "source"); e != nil {
		t.Fatal(e)
	}
	src := filepath.Join(store, "source")
	if e := os.Mkdir(filepath.Join(src, "nested"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(src, "nested", "data"), []byte("family fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(src, "empty"), nil, 0600); e != nil {
		t.Fatal(e)
	}
	r, e := Backup(ctx, store, "source", backups)
	if e != nil || r.Files != 2 || r.Bytes != 14 || r.ApplicationConsistent {
		t.Fatal(r, e)
	}
	rr, e := Restore(ctx, store, "restored", r.Directory, r.ContentSHA256)
	if e != nil || !rr.Restored || rr.ContentSHA256 != r.ContentSHA256 {
		t.Fatal(rr, e)
	}
	b, e := os.ReadFile(filepath.Join(store, "restored", "nested", "data"))
	if e != nil || string(b) != "family fixture" {
		t.Fatal(string(b), e)
	}
	if _, e = Restore(ctx, store, "source", r.Directory, r.ContentSHA256); e == nil {
		t.Fatal("overwrite allowed")
	}
	if _, e = Restore(ctx, store, "bad", r.Directory, "sha256:"+strings.Repeat("0", 64)); e == nil {
		t.Fatal("wrong pin accepted")
	}
	if _, e = os.Stat(filepath.Join(store, "bad")); !os.IsNotExist(e) {
		t.Fatal("failed restore retained new tree")
	}
	if e = os.Symlink("empty", filepath.Join(src, "link")); e != nil {
		t.Fatal(e)
	}
	if _, e = Backup(ctx, store, "source", backups); e == nil {
		t.Fatal("link accepted")
	}
	entries, e := os.ReadDir(backups)
	if e != nil || len(entries) != 1 {
		t.Fatal("failed backup retained partial tree", e)
	}
}
