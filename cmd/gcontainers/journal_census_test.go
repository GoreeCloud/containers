package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/GoreeCloud/containers/internal/engine"
)

func TestPendingSummaryCLIFlagsAndIncidentExit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("rootless journal required")
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := engine.PrepareStateRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", base)
	var out bytes.Buffer
	if err := run([]string{"oci", "pending-summary"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"pendingCount": 0`) ||
		!strings.Contains(out.String(), `"runtimeStateVerified": false`) {
		t.Fatalf("misleading idle summary: %s", out.String())
	}
	if err := run([]string{"oci", "pending-summary", "--id", "a"}, &out); err == nil {
		t.Fatal("summary accepted a container ID")
	}
	_, err = engine.RunJournaledMutation(context.Background(), root, "private-a", "create", func() ([]byte, error) {
		return nil, errors.New("synthetic interrupted OCI runtime")
	})
	if !errors.Is(err, engine.ErrUnresolvedMutation) {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"oci", "pending-summary"}, &out); !errors.Is(err, engine.ErrUnresolvedMutation) {
		t.Fatalf("unresolved mutation was not a failing monitoring exit: %v", err)
	}
	if !strings.Contains(out.String(), `"recoveryRequired": true`) ||
		!strings.Contains(out.String(), `"pendingCount": 1`) ||
		strings.Contains(out.String(), "private-a") {
		t.Fatalf("private ID leaked or incident omitted: %s", out.String())
	}
}
