package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
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
		{"validate"}, {"image"}, {"image", "verify", "missing"}, {"validate", "not-a-bundle"}, {"unknown"},
		{"oci", "unknown"}, {"oci", "create", "--id", "../bad", "--experimental"},
	} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestImageImportIsExplicitAndRootless(t *testing.T) {
	var out bytes.Buffer
	source := filepath.Join(t.TempDir(), "blob")
	if err := os.WriteFile(source, []byte("synthetic bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "image-store")
	if err := run([]string{"image", "import", "--store", destination, "--file", source}, &out); err == nil {
		t.Fatal("image import without experimental consent accepted")
	}
	if os.Geteuid() == 0 {
		t.Skip("image import intentionally forbids root")
	}
	out.Reset()
	if err := run([]string{"image", "import", "--store", destination, "--file", source, "--experimental"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sha256:") {
		t.Fatalf("digest missing: %s", out.String())
	}
	if strings.Contains(out.String(), "synthetic bytes") {
		t.Fatal("source bytes leaked into CLI output")
	}
}

func TestNativeStackCommandsFailClosed(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"stack", "plan", "missing"}, &out); err == nil {
		t.Fatal("missing stack accepted")
	}
	if err := run([]string{"stack", "apply", "whatever"}, &out); err == nil {
		t.Fatal("unimplemented apply accepted")
	}
	if err := run([]string{"stack", "plan"}, &out); err == nil {
		t.Fatal("invalid invocation accepted")
	}
	document := `{
      "apiVersion":"goreecloud.containers/v1alpha1",
      "kind":"Stack",
      "metadata":{"name":"synthetic"},
      "services":[{
         "name":"api",
         "image":"sha256:__HASH__",
         "replicas":1,
         "resources":{"cpuMilli":250,"memoryBytes":33554432},
         "security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}
      }]
    }`
	document = strings.Replace(document, "__HASH__", strings.Repeat("a", 64), 1)
	file := filepath.Join(t.TempDir(), "test-stack.json")
	if err := os.WriteFile(file, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"stack", "validate", file}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"stack", "plan", file}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"dryRun": true`) || !strings.Contains(out.String(), `"operation": "would-deploy"`) {
		t.Fatal("unsafe or missing dry-run marker")
	}
}

func TestLayerInspectionCLI(t *testing.T) {
	var bytesOut bytes.Buffer
	var tarBytes bytes.Buffer
	writer := tar.NewWriter(&tarBytes)
	if err := writer.WriteHeader(&tar.Header{Name: "readme.txt", Mode: 0644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "test.tar")
	if err := os.WriteFile(filename, tarBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"image", "inspect-layer", filename, "tar"}, &bytesOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(bytesOut.String(), `"fileCount": 1`) || !strings.Contains(bytesOut.String(), `"unpackedBytes": 2`) {
		t.Fatalf("invalid read-only output: %s", bytesOut.String())
	}
	if err := run([]string{"image", "inspect-layer", filename, "tar+zstd"}, &bytesOut); err == nil {
		t.Fatal("unsupported compression accepted")
	}
}

func TestNativeStackDiffCLIIsReadOnly(t *testing.T) {
	old := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"a"},"services":[{"name":"web","image":"sha256:__HASH__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__HASH__", strings.Repeat("a", 64), 1)
	updated := strings.Replace(old, `"replicas":1`, `"replicas":2`, 1)
	base := t.TempDir()
	one := filepath.Join(base, "one.json")
	two := filepath.Join(base, "two.json")
	if err := os.WriteFile(one, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"stack", "diff", one, two}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"manualReviewRequired": true`) ||
		!strings.Contains(out.String(), `"action": "would-replace-manual-review"`) ||
		!strings.Contains(out.String(), `"dryRun": true`) {
		t.Fatalf("unsafe dry-run response: %s", out.String())
	}
	if err := run([]string{"stack", "diff", one}, &out); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestNativeStackCapacityCLI(t *testing.T) {
	document := strings.Replace(`{"apiVersion":"goreecloud.containers/v1alpha1","kind":"Stack","metadata":{"name":"a"},"services":[{"name":"web","image":"sha256:__HASH__","replicas":1,"resources":{"cpuMilli":250,"memoryBytes":33554432},"security":{"runAsNonRoot":true,"readOnlyRootFilesystem":true,"network":"isolated"}}]}`, "__HASH__", strings.Repeat("a", 64), 1)
	file := filepath.Join(t.TempDir(), "stack.json")
	if err := os.WriteFile(file, []byte(document), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"stack", "capacity", file, "--cpu-milli", "500", "--memory-mib", "64", "--max-instances", "4"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deploymentAuthorized": false`) || !strings.Contains(out.String(), `"withinLimits": true`) {
		t.Fatal("capacity output should never authorize deployment")
	}
	out.Reset()
	if err := run([]string{"stack", "capacity", file, "--cpu-milli", "100", "--memory-mib", "16", "--max-instances", "1"}, &out); err == nil {
		t.Fatal("oversubscription must fail")
	}
	if !strings.Contains(out.String(), `"withinLimits": false`) {
		t.Fatal("failed check must contain structured denial")
	}
	out.Reset()
	if err := run([]string{"stack", "capacity", file}, &out); err == nil {
		t.Fatal("missing explicit host budgets accepted")
	}
}

func TestHostInspectionIsAdvisoryOnly(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"host", "inspect"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"readOnly": true`) ||
		!strings.Contains(out.String(), `"deploymentAuthorized": false`) ||
		!strings.Contains(out.String(), `"delegationVerified": false`) {
		t.Fatalf("unsafe host capability report: %s", out.String())
	}
	if err := run([]string{"host", "inspect", "extra"}, &out); err == nil {
		t.Fatal("unknown host command flag accepted")
	}
}
