package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Synthetic launcher deliberately leaves a short-lived child holding
// inherited stdio. It must not cause an OCI mutation to fail on pipe wait.
func TestMutationDoesNotWaitForGuestStdio(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "synthetic-launcher")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\nsleep 3 &\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := (ProcessRunner{}).Run(ctx, helper, "--root", "/synthetic", "create", "worker"); err != nil {
		t.Fatalf("successful mutation incorrectly held by child stdio: %v", err)
	}
}
