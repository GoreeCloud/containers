package main

import (
	"bytes"
	"github.com/GoreeCloud/containers/internal/stack"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevisionGuardCLIBlocksStaleSource(t *testing.T) {
	jsonText := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"alpha"},"services":[{"name":"web","image":"sha256:__HASH__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__HASH__", strings.Repeat("c", 64), 1)
	dir := t.TempDir()
	previous := filepath.Join(dir, "previous.json")
	next := filepath.Join(dir, "next.json")
	if err := os.WriteFile(previous, []byte(jsonText), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(next, []byte(strings.Replace(jsonText, `"replicas":1`, `"replicas":2`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := stack.Load(previous)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := stack.DryRun(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	args := []string{"stack", "revision-check", previous, next, "--expected-current-sha256", fingerprint.SpecSHA256}
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"matchesExpected": true`) ||
		!strings.Contains(out.String(), `"liveStateVerified": false`) ||
		!strings.Contains(out.String(), `"deploymentAuthorized": false`) {
		t.Fatalf("matching source incorrectly attested live deployment: %s", out.String())
	}
	args[5] = "sha256:" + strings.Repeat("d", 64)
	out.Reset()
	if err := run(args, &out); err == nil {
		t.Fatal("stale revision passed")
	}
	if !strings.Contains(out.String(), `"matchesExpected": false`) ||
		!strings.Contains(out.String(), `"manualReviewRequired": true`) {
		t.Fatalf("stale source conflict omitted: %s", out.String())
	}
	if err := run([]string{"stack", "revision-check", previous, next}, &out); err == nil {
		t.Fatal("missing required expected fingerprint accepted")
	}
}
