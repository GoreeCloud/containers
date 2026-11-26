package bundle

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Synthetic ELF64 with one PT_INTERP segment; the test does not execute it.
func fixtureELFInterpreter(interpreter string) []byte {
	raw := make([]byte, 256+len(interpreter)+1)
	copy(raw, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	order := binary.LittleEndian
	order.PutUint16(raw[16:], 2)   // ET_EXEC
	order.PutUint16(raw[18:], 62)  // EM_X86_64
	order.PutUint32(raw[20:], 1)   // EV_CURRENT
	order.PutUint64(raw[32:], 64)  // program header offset
	order.PutUint16(raw[52:], 64)  // ELF header size
	order.PutUint16(raw[54:], 56)  // program header entry size
	order.PutUint16(raw[56:], 1)   // one program header
	order.PutUint32(raw[64:], 3)   // PT_INTERP
	order.PutUint32(raw[68:], 4)   // PF_R
	order.PutUint64(raw[72:], 256) // interpreter offset
	order.PutUint64(raw[96:], uint64(len(interpreter)+1))
	order.PutUint64(raw[104:], uint64(len(interpreter)+1))
	order.PutUint64(raw[112:], 1)
	copy(raw[256:], interpreter)
	return raw
}

func TestELFGuestRequiresLoaderWithinRootfs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "app"), fixtureELFInterpreter("/lib64/ld-test.so"), 0700); err != nil {
		t.Fatal(err)
	}
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	if err := validateGuestELFInterpreter(guest, "bin/app"); err == nil {
		t.Fatal("missing guest ELF interpreter accepted")
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib64"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib64", "ld-test.so"), []byte("synthetic loader"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestELFInterpreter(guest, "bin/app"); err != nil {
		t.Fatalf("guest interpreter found inside rootfs but rejected: %v", err)
	}
	if err := os.Chmod(filepath.Join(dir, "lib64", "ld-test.so"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestELFInterpreter(guest, "bin/app"); err == nil {
		t.Fatal("non-executable guest loader accepted")
	}
}

func TestMalformedELFInterpreterFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad"), []byte{0x7f, 'E', 'L', 'F', 2, 1}, 0700); err != nil {
		t.Fatal(err)
	}
	guest, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	if err := validateGuestELFInterpreter(guest, "bad"); err == nil {
		t.Fatal("truncated ELF header accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := validateGuestELFInterpreter(guest, "script"); err != nil {
		t.Fatalf("non-ELF entrypoint behavior should remain unchanged: %v", err)
	}
}
