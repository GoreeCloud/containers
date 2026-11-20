package bundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuestEntrypointRejectsUnresolvedEnvShebangs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "usr", "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"usr/bin/env", "bin/sh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("synthetic executable"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	unsafe := []string{
		"#!/usr/bin/env python3\n",
		"#!/usr/bin/env -S python3 -I\n",
		"#!/bin/env -S missing\n",
	}
	for _, header := range unsafe {
		if err := os.WriteFile(filepath.Join(dir, "bin", "entry"), []byte(header), 0700); err != nil {
			t.Fatal(err)
		}
		if err := validateGuestEntrypoint(guest, "bin/entry"); err == nil {
			t.Fatalf("unresolved env secondary interpreter admitted: %q", header)
		}
	}
	// Direct absolute interpreter paths remain accepted.
	if err := os.WriteFile(filepath.Join(dir, "bin", "entry"), []byte("#!/bin/sh -e\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestEntrypoint(guest, "bin/entry"); err != nil {
		t.Fatalf("direct absolute guest interpreter unexpectedly rejected: %v", err)
	}
	// The same limitation applies at an intermediate script interpreter.
	if err := os.WriteFile(filepath.Join(dir, "bin", "entry"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "sh"), []byte("#!/usr/bin/env missing\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestEntrypoint(guest, "bin/entry"); err == nil {
		t.Fatal("nested unresolved env shebang admitted")
	}
}
