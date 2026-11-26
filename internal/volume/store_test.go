package volume

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
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

func TestVolumeCapacityConcurrent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	p := t.TempDir()
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < MaxVolumes-1; i++ {
		if e := os.Mkdir(filepath.Join(p, fmt.Sprintf("v%04d", i)), 0700); e != nil {
			t.Fatal(e)
		}
	}
	results := make(chan error, 2)
	for _, id := range []string{"last-a", "last-b"} {
		go func(id string) { _, e := Create(context.Background(), p, id); results <- e }(id)
	}
	successes := 0
	for range 2 {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("capacity race", successes)
	}
	if n, e := Count(context.Background(), p); e != nil || n != MaxVolumes {
		t.Fatal(n, e)
	}
}
func TestVolumeLockCancellation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("nonroot only")
	}
	p := t.TempDir()
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	r, e := open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	lock, e := lockStore(context.Background(), r, syscall.LOCK_EX)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	if _, e := Create(ctx, p, "blocked"); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(p, "blocked")); !os.IsNotExist(e) {
		t.Fatal("mutation despite timeout", e)
	}
}
