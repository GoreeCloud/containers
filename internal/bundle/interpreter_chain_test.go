package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuestEntrypointChecksNestedShebangChain(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	mustWrite := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "bin", name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("app", "#!/bin/launcher\n")
	mustWrite("launcher", "#!/bin/missing\n")
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	if err := validateGuestEntrypoint(guest, "bin/app"); err == nil {
		t.Fatal("missing nested script interpreter accepted")
	}
	mustWrite("missing", "synthetic executable")
	if err := validateGuestEntrypoint(guest, "bin/app"); err != nil {
		t.Fatalf("valid bounded interpreter chain rejected: %v", err)
	}
	mustWrite("missing", "#!/bin/app\n")
	if err := validateGuestEntrypoint(guest, "bin/app"); err == nil {
		t.Fatal("cyclic shebang chain accepted")
	}
}

func TestGuestEntrypointRejectsOverdeepInterpreterChain(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	names := []string{"a", "b", "c", "d", "e"}
	for i, name := range names {
		body := "synthetic executable"
		if i+1 < len(names) {
			body = "#!/bin/" + names[i+1] + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	if err := validateGuestEntrypoint(guest, "bin/a"); err == nil {
		t.Fatal("overdepth script interpreter chain accepted")
	}
}
