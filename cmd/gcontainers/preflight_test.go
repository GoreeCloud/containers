package main

import (
	"bytes"
	"github.com/GoreeCloud/containers/internal/stack"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCombinedPreflightCLIStaleAndOvercommit(t *testing.T) {
	doc := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"demo"},"services":[{"name":"web","image":"sha256:__DIGEST__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__DIGEST__", strings.Repeat("d", 64), 1)
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	target := filepath.Join(dir, "new.json")
	if err := os.WriteFile(old, []byte(doc), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(strings.Replace(doc, `"replicas":1`, `"replicas":2`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	src, err := stack.Load(old)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stack.DryRun(src)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"stack", "preflight", old, target, "--expected-current-sha256", plan.SpecSHA256,
		"--cpu-milli", "2000", "--memory-mib", "256", "--max-instances", "8"}
	var out bytes.Buffer
	// Source and budget may pass, but a changed service still requires
	// real authenticated Identity approval from a future native executor.
	if err := run(args, &out); err == nil {
		t.Fatal("changed native stack accepted without Identity grant")
	}
	if !strings.Contains(out.String(), `"goreeCloudIdentityReviewRequired": true`) ||
		!strings.Contains(out.String(), `"mipsAuthorizationVerified": false`) {
		t.Fatalf("operator approval gate omitted: %s", out.String())
	}
	if !strings.Contains(out.String(), `"sourceAndBudgetChecksPassed": true`) ||
		!strings.Contains(out.String(), `"deploymentAuthorized": false`) ||
		!strings.Contains(out.String(), `"liveStateVerified": false`) {
		t.Fatalf("preflight misreported launch authorization: %s", out.String())
	}
	args[5] = "sha256:" + strings.Repeat("e", 64)
	out.Reset()
	if err := run(args, &out); err == nil {
		t.Fatal("stale expected revision accepted")
	}
	if !strings.Contains(out.String(), `"sourceAndBudgetChecksPassed": false`) {
		t.Fatalf("stale conflict not recorded: %s", out.String())
	}
	args[5] = plan.SpecSHA256
	args[7] = "100"
	out.Reset()
	if err := run(args, &out); err == nil {
		t.Fatal("overcommitted stack unexpectedly passed preflight")
	}
	if !strings.Contains(out.String(), `"withinBudget": false`) {
		t.Fatalf("capacity failure omitted: %s", out.String())
	}
	if err := run([]string{"stack", "preflight", old, target}, &out); err == nil {
		t.Fatal("preflight without explicit hash/budget accepted")
	}
}
