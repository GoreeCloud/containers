package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoreeCloud/containers/internal/stack"
)

func TestSensitivePreflightCLIRejectsUnreviewedSecretChanges(t *testing.T) {
	document := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"demo"},"services":[{"name":"web","image":"sha256:__HASH__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__HASH__", strings.Repeat("a", 64), 1)
	target := strings.Replace(document, `"resources":`, `"secretRefs":["private-only-synthetic"],"resources":`, 1)
	dir := t.TempDir()
	prior := filepath.Join(dir, "prior.json")
	changed := filepath.Join(dir, "changed.json")
	if err := os.WriteFile(prior, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte(target), 0600); err != nil {
		t.Fatal(err)
	}
	parsed, err := stack.Load(prior)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := stack.DryRun(parsed)
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"stack", "preflight", prior, changed, "--expected-current-sha256", plan.SpecSHA256,
		"--cpu-milli", "4000", "--memory-mib", "1024", "--max-instances", "16"}
	var out bytes.Buffer
	if err := run(args, &out); err == nil {
		t.Fatal("unapproved private reference change incorrectly passed preflight")
	}
	response := out.String()
	for _, required := range []string{
		`"sourceAndBudgetChecksPassed": true`,
		`"sensitivePolicyReviewRequired": true`,
		`"secretReferenceChangeCount": 1`,
		`"goreeCloudIdentityReviewRequired": true`,
		`"goreeShieldReviewRequired": true`,
		`"wardveilReviewRequired": true`,
		`"mipsAuthorizationVerified": false`,
		`"deploymentAuthorized": false`,
	} {
		if !strings.Contains(response, required) {
			t.Fatalf("missing review boundary %q in %s", required, response)
		}
	}
	if strings.Contains(response, "private-only-synthetic") {
		t.Fatal("private secret-ref name leaked to preflight output")
	}
}
