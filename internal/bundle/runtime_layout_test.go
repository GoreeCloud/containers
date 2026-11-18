package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func guestBase(t *testing.T, dir string) {
	t.Helper()
	for _, part := range []string{"dev", "proc", "bin"} {
		if err := os.MkdirAll(filepath.Join(dir, "rootfs", part), 0700); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRuntimeLayoutRequiresGuestExecutableAndWorkingDirectory(t *testing.T) {
	dir := setup(t, safeSpec)
	guestBase(t, dir)
	spec, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("missing guest executable accepted")
	}
	bin := filepath.Join(dir, "rootfs/bin/true")
	if err := os.WriteFile(bin, []byte("test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("non-executable guest file accepted")
	}
	if err := os.Chmod(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err != nil {
		t.Fatalf("valid file layout refused: %v", err)
	}
	raw := strings.Replace(safeSpec, "\"cwd\":\"/\"", "\"cwd\":\"/missing\"", 1)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("missing guest working directory accepted")
	}
}
func TestRuntimeLayoutRejectsEntrypointSymlinkEscape(t *testing.T) {
	dir := setup(t, safeSpec)
	guestBase(t, dir)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "rootfs/bin/true")); err != nil {
		t.Fatal(err)
	}
	spec, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeLayout(dir, spec); err == nil {
		t.Fatal("guest followed host symlink")
	}
}
