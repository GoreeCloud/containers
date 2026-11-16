package kernelprobe

import (
	"context"
	"errors"
	"testing"
)

func TestCompareDistinctAndIncompleteKernelNamespaces(t *testing.T) {
	parent, child := map[string]string{}, map[string]string{}
	for _, n := range names {
		parent[n] = "before-" + n
		child[n] = "after-" + n
	}
	if !Compare(parent, child, 0) {
		t.Fatal("distinct mapped namespaces rejected")
	}
	if Compare(parent, child, 1000) {
		t.Fatal("unmapped identity accepted")
	}
	child["net"] = parent["net"]
	if Compare(parent, child, 0) {
		t.Fatal("shared network namespace accepted")
	}
	delete(child, "net")
	if Compare(parent, child, 0) {
		t.Fatal("missing namespace accepted")
	}
}
func TestCanceledKernelProbeDoesNotAuthorizeDeployment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := Probe(ctx, "/usr/bin/true")
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if !report.ReadOnly || report.DeploymentAuthorized || report.OCIIsolationVerified ||
		report.SeccompVerified || report.LSMVerified || report.CgroupDelegationVerified {
		t.Fatal("unsupported security certification")
	}
}
