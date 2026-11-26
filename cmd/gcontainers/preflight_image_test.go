package main

import (
	"bytes"
	"github.com/GoreeCloud/containers/internal/stack"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightRejectsImageDigestChangeAndRedactsReport(t *testing.T) {
	src := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"offline"},"services":[{"name":"web","image":"sha256:__HASH__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__HASH__", strings.Repeat("a", 64), 1)
	target := strings.Replace(src, strings.Repeat("a", 64), strings.Repeat("b", 64), 1)
	dir := t.TempDir()
	before := filepath.Join(dir, "old.json")
	after := filepath.Join(dir, "changed.json")
	if err := os.WriteFile(before, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(after, []byte(target), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := stack.Load(before)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stack.DryRun(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	args := []string{"stack", "preflight", before, after,
		"--expected-current-sha256", plan.SpecSHA256,
		"--cpu-milli", "4000", "--memory-mib", "1024", "--max-instances", "16"}
	if err := run(args, &output); err == nil {
		t.Fatal("image change passed without independent MIPS review")
	}
	for _, expected := range []string{
		`"sourceAndBudgetChecksPassed": true`,
		`"imageReferenceChangeCount": 1`,
		`"imageProvenanceVerified": false`,
		`"wardveilReviewRequired": true`,
		`"goreeShieldReviewRequired": true`,
		`"sensitivePolicyReviewRequired": true`,
		`"deploymentAuthorized": false`,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("missing %q in review: %s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), strings.Repeat("b", 64)) {
		t.Fatal("changed image descriptor leaked via embedded impact details")
	}
}
