package volume

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentDataAndNoOverwrite(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	p := t.TempDir()
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	r, e := Create(ctx, p, "family")
	if e != nil || !r.Private || r.Mounted {
		t.Fatal(r, e)
	}
	file := filepath.Join(p, "family", "sentinel")
	if e = os.WriteFile(file, []byte("synthetic"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Create(ctx, p, "family"); e == nil {
		t.Fatal("existing volume reset")
	}
	got, e := os.ReadFile(file)
	if e != nil || string(got) != "synthetic" {
		t.Fatal("lost volume content", e)
	}
	if n, e := Count(ctx, p); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if r, e = Inspect(ctx, p, "family"); e != nil || !r.Exists {
		t.Fatal(r, e)
	}
	for _, id := range []string{"../outside", "-bad", "a/b"} {
		if _, e = Create(ctx, p, id); e == nil {
			t.Fatal("unsafe ID")
		}
	}
	if e = os.Symlink("family", filepath.Join(p, "alias")); e != nil {
		t.Fatal(e)
	}
	if _, e = Inspect(ctx, p, "alias"); e == nil {
		t.Fatal("symlink accepted")
	}
	if _, e = Count(ctx, p); e == nil {
		t.Fatal("unsafe inventory accepted")
	}
}
