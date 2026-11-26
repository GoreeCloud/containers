package ociimage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLayoutReadIdentityRejectsSameSizeSwap(t *testing.T) {
	dir := t.TempDir()
	filename := "blob"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte("abcd"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before, err := root.Lstat(filename)
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := verifyLayoutReadIdentity(root, file, before, filename); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(filename); err != nil {
		t.Fatal(err)
	}
	other, err := root.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := verifyLayoutReadIdentity(root, file, before, filename); err == nil {
		t.Fatal("same-length OCI blob replacement accepted")
	}
}
func TestLayoutReadIdentityRejectsSymlinkAndChangedLength(t *testing.T) {
	dir := t.TempDir()
	name := "index.json"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	before, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := os.WriteFile(path, []byte("{\"x\":1}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyLayoutReadIdentity(root, file, before, name); err == nil {
		t.Fatal("metadata size change accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", path); err != nil {
		t.Fatal(err)
	}
	if err := verifyLayoutReadIdentity(root, file, before, name); err == nil {
		t.Fatal("symlink metadata replacement accepted")
	}
}
