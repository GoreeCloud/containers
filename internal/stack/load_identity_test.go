package stack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStackSourceIdentityRejectsSameLengthReplacedPath(t *testing.T) {
	dir := t.TempDir()
	name := "source.json"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	initial, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := verifyStackInputIdentity(root, f, initial, name); err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	other, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := verifyStackInputIdentity(root, f, initial, name); err == nil {
		t.Fatal("same-size stack document pathname substitution accepted")
	}
}
func TestStackSourceIdentityRejectsSymlinkAndInPlaceResize(t *testing.T) {
	dir := t.TempDir()
	name := "old.json"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	initial, err := root.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := os.WriteFile(path, []byte("{ }"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyStackInputIdentity(root, f, initial, name); err == nil {
		t.Fatal("changed stack length accepted")
	}
	if err := root.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", path); err != nil {
		t.Fatal(err)
	}
	if err := verifyStackInputIdentity(root, f, initial, name); err == nil {
		t.Fatal("symlink stack source replacement accepted")
	}
}
