package bundle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func snapshotFixture(t *testing.T) string {
	t.Helper()
	source := setup(t, strings.Replace(strings.Replace(strings.Replace(safeSpec, `"hostID":1000`, fmt.Sprintf(`"hostID":%d`, os.Getuid()), 1), `"hostID":1000`, fmt.Sprintf(`"hostID":%d`, os.Getgid()), 1), `"defaultAction":"SCMP_ACT_ERRNO"`, `"defaultAction":"SCMP_ACT_ERRNO","syscalls":[{"names":["execve"],"action":"SCMP_ACT_ALLOW"}]`, 1))
	for _, name := range []string{"dev", "proc", "bin"} {
		if err := os.Mkdir(filepath.Join(source, "rootfs", name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(source, 0700); err != nil {
		t.Fatal(err)
	}
	// A synthetic ELF-independent file is sufficient for layout preflight.
	if err := os.WriteFile(filepath.Join(source, "rootfs/bin/true"), []byte("inert fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	return source
}
func TestSnapshotRestrictedCopy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless only")
	}
	source := snapshotFixture(t)
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	a, err := Snapshot(context.Background(), source, parent)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Snapshot(context.Background(), source, parent)
	if err != nil {
		t.Fatal(err)
	}
	if a.ContentSHA256 != b.ContentSHA256 || a.Directory == b.Directory || a.Immutable || a.ExecutionAuthorized {
		t.Fatalf("bad snapshot %+v %+v", a, b)
	}
	if err = os.WriteFile(filepath.Join(source, "rootfs/bin/true"), []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(a.Directory, "rootfs/bin/true"))
	if err != nil || string(data) != "inert fixture" {
		t.Fatal("copy depends on mutable source", err)
	}
}
func TestSnapshotRejectsLinksAndCleans(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless only")
	}
	for _, link := range []string{"symlink", "hardlink"} {
		t.Run(link, func(t *testing.T) {
			source := snapshotFixture(t)
			parent := t.TempDir()
			if err := os.Chmod(parent, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(source, "rootfs/bin/true")
			alias := filepath.Join(source, "rootfs/bin/alias")
			var err error
			if link == "symlink" {
				err = os.Symlink(target, alias)
			} else {
				err = os.Link(target, alias)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Snapshot(context.Background(), source, parent); err == nil {
				t.Fatal("link accepted")
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed snapshot retained tree", err)
			}
		})
	}
}
