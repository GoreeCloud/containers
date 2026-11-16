package main

import (
	"bytes"
	"github.com/GoreeCloud/containers/internal/engine"
	"os"
	"strings"
	"testing"
)

func TestReadOnlyCommands(t *testing.T) {
	var b bytes.Buffer
	if err := run([]string{"version"}, &b); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(b.String()) != version {
		t.Fatal(b.String())
	}
	b.Reset()
	if err := run([]string{"doctor"}, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"production_ready": false`) {
		t.Fatal(b.String())
	}
}
func TestInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"validate"}, {"validate", "not-a-bundle"}, {"unknown"},
		{"oci", "unknown"}, {"oci", "create", "--id", "../bad", "--experimental"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestPendingOCICommandRequiresNoExternalRuntime(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless journal required")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.PrepareStateRoot(base); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", base)
	t.Setenv("PATH", "/usr/bin:/bin")
	var out bytes.Buffer
	if err := run([]string{"oci", "pending", "--id", "worker"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pending": false`) ||
		!strings.Contains(out.String(), `"readOnly": true`) ||
		!strings.Contains(out.String(), `"recoveryRequired": false`) {
		t.Fatalf("unsafe journal inspection: %s", out.String())
	}
	if err := run([]string{"oci", "pending", "--id", "worker", "--experimental"}, &out); err == nil {
		t.Fatal("mutation flag accepted on read-only journal command")
	}
}
