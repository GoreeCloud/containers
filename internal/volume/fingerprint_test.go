package volume

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintMatchesBackupAndRejectsChange(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless store")
	}
	ctx := context.Background()
	store, parent := t.TempDir(), t.TempDir()
	for _, p := range []string{store, parent} {
		if e := os.Chmod(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := Create(ctx, store, "data"); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(store, "data", "payload")
	if e := os.WriteFile(file, []byte("fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := Fingerprint(ctx, store, "data", "")
	if e != nil || f.DigestMatches || !f.ReadOnly || f.MountVerified || f.ApplicationConsistent {
		t.Fatal(f, e)
	}
	backup, e := Backup(ctx, store, "data", parent)
	if e != nil || backup.ContentSHA256 != f.ContentSHA256 {
		t.Fatal(backup, e)
	}
	if r, e := Fingerprint(ctx, store, "data", f.ContentSHA256); e != nil || !r.DigestMatches {
		t.Fatal(r, e)
	}
	if e := os.WriteFile(file, []byte("changed"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Fingerprint(ctx, store, "data", f.ContentSHA256); e == nil {
		t.Fatal("changed content accepted")
	}
	if e := os.Symlink(file, filepath.Join(store, "data", "alias")); e != nil {
		t.Fatal(e)
	}
	if _, e := Fingerprint(ctx, store, "data", ""); e == nil {
		t.Fatal("symlink accepted")
	}
}
