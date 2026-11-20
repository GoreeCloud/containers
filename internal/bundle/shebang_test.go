package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShebangGuestRequiresInterpreterInsideRootfs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "bin", "start")
	if err := os.WriteFile(script, []byte("#!/bin/sh -e\necho test\n"), 0700); err != nil {
		t.Fatal(err)
	}
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	if err := validateGuestScriptInterpreter(guest, "bin/start"); err == nil {
		t.Fatal("missing interpreter accepted")
	}
	loader := filepath.Join(dir, "bin", "sh")
	if err := os.WriteFile(loader, []byte("synthetic shell"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestScriptInterpreter(guest, "bin/start"); err != nil {
		t.Fatalf("present executable interpreter rejected: %v", err)
	}
	if err := os.Chmod(loader, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestScriptInterpreter(guest, "bin/start"); err == nil {
		t.Fatal("non-executable interpreter accepted")
	}
}

func TestShebangGuestRejectsUnsafeHeader(t *testing.T) {
	dir := t.TempDir()
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	bad := []string{
		"#!../bin/sh\n",
		"#! /bin/../sh\n",
		"#!\n",
		"#!/bin/sh\r\n",
		"#!/" + strings.Repeat("a", 260) + "\n",
	}
	for i, value := range bad {
		if err := os.WriteFile(filepath.Join(dir, "script"), []byte(value), 0700); err != nil {
			t.Fatal(err)
		}
		if err := validateGuestScriptInterpreter(guest, "script"); err == nil {
			t.Fatalf("unsafe shebang case %d accepted", i)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "plain"), []byte("non-ELF non-script"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestScriptInterpreter(guest, "plain"); err != nil {
		t.Fatalf("plain non-script should retain current behavior: %v", err)
	}
}
