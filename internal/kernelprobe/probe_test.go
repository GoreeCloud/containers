package kernelprobe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestCompareDistinctAndIncompleteKernelNamespaces(t *testing.T) {
	parent, child := map[string]string{}, map[string]string{}
	for _, n := range names {
		parent[n] = n + ":[123000]"
		child[n] = n + ":[123001]"
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

func TestProbeChildUsesBoundedInputAndNeverLeaksNamespaceIdentifiers(t *testing.T) {
	if _, err := WriteChildComparison(bytes.NewReader(nil)); err == nil {
		t.Fatal("child accepted direct invocation without a parent")
	}
	if _, err := WriteChildComparison(bytes.NewReader(bytes.Repeat([]byte("x"), 2200))); err == nil {
		t.Fatal("child accepted oversized parent data")
	}
	parent, _, err := ReadSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	source, err := json.Marshal(snapshot{IDs: parent, UID: 1000})
	if err != nil {
		t.Fatal(err)
	}
	result, err := WriteChildComparison(bytes.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(result, []byte("user:[")) ||
		bytes.Contains(result, []byte("net:[")) ||
		bytes.Contains(result, []byte(`"ids"`)) {
		t.Fatalf("child output included raw namespace identifiers: %s", result)
	}
	var comparison childComparison
	if err := json.Unmarshal(result, &comparison); err != nil {
		t.Fatal(err)
	}
	if comparison.NamespacesDistinct || comparison.UserMappedAsRoot {
		t.Fatal("self namespace was incorrectly claimed distinct")
	}
}
